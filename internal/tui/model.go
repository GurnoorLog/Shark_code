package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"shark-agent/internal/agent"
	"shark-agent/internal/config"
	"shark-agent/internal/provider"
)

type entry struct {
	kind  string // "user", "assistant", "tool", "tool_result", "error", "system", "help"
	text  string
}

type model struct {
	cfg     *config.Config
	reg     *provider.Registry
	ag      *agent.Agent
	ctx     context.Context
	cancel  context.CancelFunc
	entries []entry
	input   string
	width   int
	height  int
	busy    bool
	spinner int
	water   *Water

	// picker state (opencode-style model selector)
	picker   bool
	pickIdx  int
	pickOnes []string
	pickTwo  map[string][]string
}

type tickMsg time.Time

type turnDoneMsg struct {
	events []agent.TurnEvent
	err    error
}

type pickMsg struct{ idx int }

func New(cfg *config.Config, reg *provider.Registry, ag *agent.Agent) *model {
	m := &model{cfg: cfg, reg: reg, ag: ag}
	m.pickTwo = map[string][]string{
		"openai":    {"gpt-4o", "gpt-4o-mini", "gpt-4.1"},
		"anthropic": {"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-4-20250514"},
		"gemini":    {"gemini-2.0-flash", "gemini-2.0-pro", "gemini-2.5-flash"},
		"fireworks": {"accounts/fireworks/models/minimax-m3", "accounts/fireworks/models/kimi-k2p7-code", "accounts/fireworks/models/gemma-4-31b-it"},
		"local":     {"gemma-2-2b-it", "llama-3.2-3b-instruct"},
	}
	return m
}

func (m *model) Init() tea.Cmd {
	m.entries = append(m.entries, entry{kind: "system", text: "SHARKCODE — the terminal coding agent. Type /help or press Tab for models."})
	return tea.Batch(
		tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
		tea.WindowSize(),
	)
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.water = NewWater(msg.Width, msg.Height)
		return m, nil

	case tickMsg:
		if m.water != nil {
			m.water.Tick()
		}
		if m.busy {
			m.spinner++
		}
		return m, tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })

	case turnDoneMsg:
		for _, ev := range msg.events {
			m.pushEvent(ev)
		}
		if msg.err != nil {
			m.entries = append(m.entries, entry{kind: "error", text: msg.err.Error()})
		}
		m.busy = false
		return m, nil

	case pickMsg:
		return m, m.handlePick(msg.idx)

	case tea.KeyMsg:
		// While the picker is open, intercept navigation keys.
		if m.picker {
			switch msg.String() {
			case "up", "k":
				if m.pickIdx > 0 {
					m.pickIdx--
				}
			case "down", "j":
				if m.pickIdx < len(m.pickOnes)-1 {
					m.pickIdx++
				}
			case "tab", "enter", " ":
				return m, pickCmd(m.pickIdx)
			case "esc", "q":
				m.picker = false
			}
			return m, nil
		}

		switch msg.String() {
		case "ctrl+c":
			if m.busy {
				if m.cancel != nil {
					m.cancel()
				}
				m.busy = false
				m.entries = append(m.entries, entry{kind: "system", text: "shark dove back into the depths (cancelled)"})
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			line := strings.TrimSpace(m.input)
			m.input = ""
			if line == "" {
				return m, nil
			}
			return m, m.submit(line)
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		case "tab":
			m.openPicker()
			return m, nil
		case "esc":
			m.input = ""
			return m, nil
		default:
			key := msg.String()
			if len(key) == 1 {
				m.input += key
				return m, nil
			}
			return m, nil
		}
	}

	return m, nil
}

func pickCmd(idx int) tea.Cmd {
	return func() tea.Msg { return pickMsg{idx} }
}

// openPicker starts provider selection.
func (m *model) openPicker() {
	m.picker = true
	m.pickIdx = 0
	m.pickOnes = providerNames(m.cfg)
}

// handlePick resolves a picker selection: first provider, then model.
func (m *model) handlePick(idx int) tea.Cmd {
	if idx < 0 || idx >= len(m.pickOnes) {
		m.picker = false
		return nil
	}
	sel := m.pickOnes[idx]

	// If we just selected a provider, show its model list next.
	if _, isProvider := m.cfg.Providers[sel]; isProvider {
		m.cfg.ActiveProvider = sel
		if models, ok := m.pickTwo[sel]; ok {
			m.pickOnes = models
			m.pickIdx = 0
			m.picker = true
			return nil
		}
		m.cfg.Save()
		m.entries = append(m.entries, entry{kind: "system", text: "switched to provider: " + sel})
		return nil
	}

	// Otherwise the selection is a model name for the active provider.
	m.cfg.Active().Model = sel
	m.cfg.Save()
	m.picker = false
	m.entries = append(m.entries, entry{kind: "system", text: fmt.Sprintf("switched to %s (%s)", m.cfg.ActiveProvider, sel)})
	return nil
}

func isPrintable(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

func (m *model) submit(line string) tea.Cmd {
	if strings.HasPrefix(line, "/") {
		return m.handleCommand(line)
	}

	m.entries = append(m.entries, entry{kind: "user", text: line})
	m.busy = true
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.ctx, m.cancel = ctx, cancel

	return func() tea.Msg {
		defer cancel()
		var events []agent.TurnEvent
		_, err := m.ag.Turn(ctx, line, func(ev agent.TurnEvent) {
			events = append(events, ev)
		})
		return turnDoneMsg{events: events, err: err}
	}
}

func (m *model) pushEvent(ev agent.TurnEvent) {
	switch ev.Type {
	case "tool":
		m.entries = append(m.entries, entry{kind: "tool", text: "fins up → " + ev.Tool + " " + truncate(ev.Args, 80)})
	case "tool_result":
		m.entries = append(m.entries, entry{kind: "tool_result", text: truncate(ev.Result, 300)})
	case "answer":
		m.entries = append(m.entries, entry{kind: "assistant", text: ev.Text})
	case "error":
		m.entries = append(m.entries, entry{kind: "error", text: ev.Text})
	}
}

func (m *model) handleCommand(line string) tea.Cmd {
	parts := strings.Fields(line)
	cmd := strings.ToLower(parts[0])

	switch cmd {
	case "/help":
		m.entries = append(m.entries, entry{kind: "help", text: helpText()})

	case "/model":
		if len(parts) < 2 {
			m.openPicker()
			return nil
		}
		name := parts[1]
		if _, ok := m.cfg.Providers[name]; !ok {
			m.entries = append(m.entries, entry{kind: "error", text: "unknown provider: " + name + " (use /model to pick)"})
			return nil
		}
		m.cfg.ActiveProvider = name
		if len(parts) >= 3 {
			m.cfg.Providers[name].Model = parts[2]
		}
		m.cfg.Save()
		p := m.cfg.Providers[name]
		m.entries = append(m.entries, entry{kind: "system", text: fmt.Sprintf("switched to %s (%s)", name, p.Model)})

	case "/key":
		if len(parts) < 3 {
			m.entries = append(m.entries, entry{kind: "error", text: "usage: /key <provider> <API_KEY>"})
			return nil
		}
		name := parts[1]
		if _, ok := m.cfg.Providers[name]; !ok {
			m.entries = append(m.entries, entry{kind: "error", text: "unknown provider: " + name})
			return nil
		}
		m.cfg.Providers[name].APIKey = parts[2]
		m.cfg.Save()
		m.reg = provider.NewRegistry(m.cfg)
		m.entries = append(m.entries, entry{kind: "system", text: fmt.Sprintf("API key saved for %s", name)})

	case "/clear":
		m.entries = nil
		m.ag.Clear()
		m.entries = append(m.entries, entry{kind: "system", text: "SHARKCODE — the terminal coding agent."})

	case "/providers":
		var b strings.Builder
		for _, n := range providerNames(m.cfg) {
			p := m.cfg.Providers[n]
			marker := "  "
			if n == m.cfg.ActiveProvider {
				marker = "▶ "
			}
			key := ""
			if p.APIKey != "" {
				key = " ✓"
			}
			fmt.Fprintf(&b, "%s%s: %s%s\n", marker, n, p.Model, key)
		}
		m.entries = append(m.entries, entry{kind: "system", text: strings.TrimSuffix(b.String(), "\n")})

	case "/exit", "/quit":
		return tea.Quit

	default:
		m.entries = append(m.entries, entry{kind: "error", text: "unknown command: " + cmd + " (try /help)"})
	}
	return nil
}

func providerNames(cfg *config.Config) []string {
	order := []string{"openai", "anthropic", "gemini", "fireworks", "local"}
	var out []string
	for _, o := range order {
		if _, ok := cfg.Providers[o]; ok {
			out = append(out, o)
		}
	}
	return out
}

func helpText() string {
	return `commands:
  /model                 open the interactive model picker (↑/↓, Enter)
  /model <provider> [model]   switch provider, optionally set model
  /key <provider> <KEY>  save an API key
  /providers             list providers and their models
  /clear                 clear conversation
  /help                  this help
  /exit                  quit

shortcuts:
  Tab                    open the model picker
  Esc                    clear the input line

providers: openai, anthropic, gemini, fireworks, local

example:
  /key anthropic sk-ant-xxxx
  /key gemini AIzaxxxx
  /model fireworks accounts/fireworks/models/minimax-m3
  /model local gemma-2-2b-it`
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (m *model) View() string {
	if m.water == nil {
		m.water = NewWater(80, 24)
	}
	bg := m.water.Render()

	// Header
	p := m.cfg.Active()
	modelName := "none"
	if p != nil {
		modelName = p.Model
	}
	status := fmt.Sprintf("🦈 %s %s", m.cfg.ActiveProvider, modelName)
	if m.busy {
		frames := []string{"sharking", "sharkin", "chomp", "swimming", "hunting"}
		status = fmt.Sprintf("🦈 %s %s  %s", m.cfg.ActiveProvider, frames[m.spinner%len(frames)], modelName)
	}

	// Entries
	var parts []string
	for _, e := range m.entries {
		switch e.kind {
		case "user":
			parts = append(parts, UserStyle.Render("🧑 you:\n"+e.text))
		case "assistant":
			parts = append(parts, AssistantStyle.Render("🦈 shark:\n"+e.text))
		case "tool":
			parts = append(parts, ToolStyle.Render("🔧 " + e.text))
		case "tool_result":
			parts = append(parts, ToolStyle.Render("📦 " + e.text))
		case "error":
			parts = append(parts, ErrorStyle.Render("⚠ " + e.text))
		case "system":
			parts = append(parts, HintStyle.Render("· " + e.text))
		case "help":
			parts = append(parts, AssistantStyle.Render(e.text))
		}
	}
	content := strings.Join(parts, "\n\n")

	// Input
	input := InputStyle.Render("> " + m.input + "▌")
	statusBar := PromptStyle.Render("sharkcode") + " " + input + "\n" +
		HintStyle.Render("Tab: models · /help · /providers · /exit")

	// Assemble the layered view over the water.
	contentLines := strings.Split(content, "\n")
	headerLine := HeaderStyle.Render(" " + status)
	statusLines := strings.Split(statusBar, "\n")

	viewport := m.height - 4
	// content lines already include blank separators.
	overlay := []string{headerLine}
	overlay = append(overlay, contentLines...)
	overlay = append(overlay, statusLines...)

	// Scroll: keep the newest `viewport` overlay lines.
	if len(overlay) > viewport {
		overlay = overlay[len(overlay)-viewport:]
	}

	// Composite onto background.
	for i, line := range overlay {
		if i < len(bg) {
			bg[i] = line
		}
	}

	// Render picker on top.
	if m.picker {
		picker := m.renderPicker()
		pickLines := strings.Split(picker, "\n")
		start := len(bg) - len(pickLines) - 1
		if start < 0 {
			start = 0
		}
		for i, pl := range pickLines {
			if start+i < len(bg) {
				bg[start+i] = pl
			}
		}
	}

	return strings.Join(bg, "\n")
}

func (m *model) renderPicker() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Foreground(Teal).Bold(true).Render("select model  (↑/↓ enter, esc cancel)\n"))

	// Are we picking providers or models?
	isProvider := len(m.pickOnes) > 0 && m.pickOnes[0] == "openai"

	if isProvider {
		for i, name := range m.pickOnes {
			marker := "  "
			label := name
			if p, ok := m.cfg.Providers[name]; ok && p.APIKey != "" {
				label += " ✓"
			}
			style := HintStyle
			if i == m.pickIdx {
				marker = "▶ "
				style = lipgloss.NewStyle().Foreground(Teal).Bold(true).Background(lipgloss.Color("#0b587a")).Padding(0, 1)
			}
			b.WriteString(style.Render(marker + label) + "\n")
		}
	} else {
		for i, model := range m.pickOnes {
			marker := "  "
			style := HintStyle
			if i == m.pickIdx {
				marker = "▶ "
				style = lipgloss.NewStyle().Foreground(Teal).Bold(true).Background(lipgloss.Color("#0b587a")).Padding(0, 1)
			}
			b.WriteString(style.Render(marker+model) + "\n")
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Teal).
		Background(lipgloss.Color("#0a3d62")).
		Padding(0, 2).
		Render(b.String())
	return box
}
