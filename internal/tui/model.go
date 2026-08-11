package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"shark-agent/internal/agent"
	"shark-agent/internal/config"
	"shark-agent/internal/provider"
)

type entry struct {
	kind string // "user", "assistant", "tool", "tool_result", "error", "system", "help"
	text string
}

type cmdInfo struct {
	name string
	desc string
}

var commands = []cmdInfo{
	{"/model", "switch provider / model (opens picker)"},
	{"/key", "save an API key  →  /key <provider> <KEY>"},
	{"/verify", "toggle the accuracy gate (on/off)"},
	{"/plan", "switch to plan mode (read-only) — tab toggles too"},
	{"/build", "switch to build mode — tab toggles too"},
	{"/providers", "list providers and their models"},
	{"/clear", "start a fresh conversation"},
	{"/help", "show all commands"},
	{"/exit", "quit sharkcode"},
}

type model struct {
	cfg     *config.Config
	reg     *provider.Registry
	ag      *agent.Agent
	ctx     context.Context
	cancel  context.CancelFunc
	entries []entry
	input   string
	autoIdx int
	width   int
	height  int
	busy    bool
	spinner int
	water   *Water

	// picker state (opencode-style model selector)
	picker     bool
	pickLocal  bool
	pickIdx    int
	pickOnes   []string
	pickTwo    map[string][]string
	localMap   map[string]string // model name -> base URL for local

	// streamCh carries agent events to the UI in real time.
	streamCh chan turnEventMsg

	// confirmState tracks an in-flight permission request.
	confirmTool  string
	confirmArgs  string
	confirmReply chan bool

	// streaming is true while answer tokens are trickling in, so pushEvent
	// accumulates them into the live assistant entry.
	streaming bool

	// scroll is the number of lines scrolled back from the bottom of the
	// chat log (0 = pinned to latest). PgUp/PgDn and mouse wheel adjust it.
	scroll int

	// escArmed lets a second Esc interrupt a running agent: the first Esc
	// shows "press esc again to cancel", the second cancels.
	escArmed bool

	// history is the ring of previously submitted prompts; histIdx indexes
	// into it for ↑/↓ recall (opencode-style).
	history []string
	histIdx int

	// plan toggles PLAN vs BUILD mode (Tab, opencode-style). In plan mode
	// the agent may only read/think and must not modify anything.
	plan bool
}

type localModelsMsg struct{ models []provider.LocalModel }

type tickMsg time.Time

type turnDoneMsg struct {
	events []agent.TurnEvent
	err    error
}

// turnEventMsg streams one agent event (or the final completion marker)
// to the UI in real time as the agent works.
type turnEventMsg struct {
	ev   agent.TurnEvent
	err  error
	done bool
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
	return tea.Batch(
		tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) }),
		tea.WindowSize(),
		tea.EnableBracketedPaste,
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

	case turnEventMsg:
		if msg.done {
			m.busy = false
			if msg.err != nil {
				m.entries = append(m.entries, entry{kind: "error", text: msg.err.Error()})
			}
			return m, nil
		}
		m.pushEvent(msg.ev)
		// keep streaming: pull the next event immediately.
		return m, m.nextStream()

	case pickMsg:
		return m, m.handlePick(msg.idx)

	case localModelsMsg:
		if m.picker && m.pickLocal && len(msg.models) > 0 {
			m.localMap = map[string]string{}
			for _, lm := range msg.models {
				m.localMap[lm.Name] = lm.BaseURL
			}
			names := make([]string, 0, len(msg.models))
			for _, lm := range msg.models {
				names = append(names, lm.Name)
			}
			m.pickOnes = names
			m.pickIdx = 0
		}
		return m, nil

	case tea.KeyMsg:
		// Bracketed paste arrives as one KeyMsg with Paste=true containing
		// the full pasted runes — append them all at once instead of one
		// key-flood at a time.
		if msg.Paste {
			if m.confirmReply != nil || m.picker {
				return m, nil
			}
			m.input += string(msg.Runes)
			m.autoIdx = 0
			return m, nil
		}
		// Permission prompt takes priority: y/enter allow, n/esc deny.
		if m.confirmReply != nil {
			var ok bool
			switch msg.String() {
			case "y", "Y", "enter":
				ok = true
			case "n", "N", "esc", "q", "ctrl+c":
				ok = false
			default:
				return m, nil
			}
			m.confirmReply <- ok
			m.confirmReply = nil
			m.entries = append(m.entries, entry{kind: "system", text: "allowed ✓"})
			if !ok {
				m.entries = append(m.entries, entry{kind: "system", text: "denied ✗"})
			}
			return m, m.nextStream()
		}

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

		// Slash-command autocomplete menu.
		if matches := m.menuMatches(); len(matches) > 0 {			switch msg.String() {
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
			case "up", "k":
				if m.autoIdx > 0 {
					m.autoIdx--
				}
				return m, nil
			case "down", "j":
				if m.autoIdx < len(matches)-1 {
					m.autoIdx++
				}
				return m, nil
			case "enter":
				m.applyCommand(matches[m.autoIdx])
				return m, nil
			case "backspace":
				if len(m.input) > 0 {
					m.input = m.input[:len(m.input)-1]
				}
				m.autoIdx = 0
				return m, nil
			case "esc":
				m.input = ""
				m.autoIdx = 0
				return m, nil
			default:
				key := msg.String()
				if len(key) == 1 {
					m.input += key
					m.autoIdx = 0
					return m, nil
				}
				return m, nil
			}
		}

		switch msg.String() {
		case "tab":
			// Plan/Build toggle (opencode-style). Allowed anytime except
			// while a permission prompt is up.
			if m.confirmReply == nil {
				m.plan = !m.plan
				m.entries = append(m.entries, entry{kind: "system", text: "switched to " + m.modeName() + " mode"})
			}
			return m, nil
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
		case "pgup":
			m.scroll += m.scrollPage()
			return m, nil
		case "pgdown":
			m.scroll -= m.scrollPage()
			if m.scroll < 0 {
				m.scroll = 0
			}
			return m, nil
		case "wheelup":
			m.scroll += 3
			return m, nil
		case "wheeldown":
			m.scroll -= 3
			if m.scroll < 0 {
				m.scroll = 0
			}
			return m, nil
		case "ctrl+up":
			m.scroll += m.scrollPage()
			return m, nil
		case "ctrl+down":
			m.scroll -= m.scrollPage()
			if m.scroll < 0 {
				m.scroll = 0
			}
			return m, nil
		case "up":
			m.recallHistory(-1)
			return m, nil
		case "down":
			m.recallHistory(1)
			return m, nil
		case "enter":
			line := strings.TrimSpace(m.input)
			m.input = ""
			m.autoIdx = 0
			if line == "" {
				return m, nil
			}
			return m, m.submit(line)
		case "backspace":
			if len(m.input) > 0 {
				m.input = m.input[:len(m.input)-1]
			}
			return m, nil
		case "esc":
			m.input = ""
			// First esc: clear input + clear any armed state. If busy and
			// already empty, arm the second-esc-to-cancel.
			if m.busy {
				if m.escArmed {
					if m.cancel != nil {
						m.cancel()
					}
					m.busy = false
					m.escArmed = false
					m.entries = append(m.entries, entry{kind: "system", text: "shark dove back into the depths (cancelled)"})
				} else {
					m.escArmed = true
					m.entries = append(m.entries, entry{kind: "system", text: "press esc again to cancel the task…"})
				}
			} else {
				m.escArmed = false
			}
			return m, nil
		default:
			key := msg.String()
			// any real keystroke disarms the double-esc.
			if len(key) == 1 {
				m.escArmed = false
				m.input += key
				return m, nil
			}
			return m, nil
		}

	case tea.MouseMsg:
		// Wheel scrolling over the log (bubbletea delivers wheel events as
		// MouseMsg, not KeyMsg). Ignored while the permission modal is up.
		if m.confirmReply != nil || m.picker {
			return m, nil
		}
		switch msg.Type {
		case tea.MouseWheelUp:
			m.scroll += 3
		case tea.MouseWheelDown:
			m.scroll -= 3
			if m.scroll < 0 {
				m.scroll = 0
			}
		}
		return m, nil
	}

	return m, nil
}

// scrollPage returns how many lines one scroll page covers.
func (m *model) scrollPage() int {
	if m.height < 8 {
		return m.height - 2
	}
	return m.height - 4
}

// recallHistory moves backward (-1, ↑) or forward (1, ↓) through earlier
// prompts, filling the input with the recalled text (opencode-style).
func (m *model) recallHistory(dir int) {
	if len(m.history) == 0 {
		return
	}
	m.histIdx += dir
	n := len(m.history)
	if m.histIdx < 0 {
		m.histIdx = 0
	}
	if m.histIdx >= n {
		m.histIdx = n // just past the newest = empty input
	}
	if m.histIdx < n {
		m.input = m.history[m.histIdx]
	} else {
		m.input = ""
	}
	m.autoIdx = 0
}

func pickCmd(idx int) tea.Cmd {
	return func() tea.Msg { return pickMsg{idx} }
}

// menuMatches returns the slash commands matching the current input.
// The menu is live while input starts with "/" and has no argument space.
func (m *model) menuMatches() []cmdInfo {
	if !strings.HasPrefix(m.input, "/") {
		return nil
	}
	if strings.Contains(m.input, " ") {
		return nil
	}
	typed := strings.ToLower(strings.TrimPrefix(m.input, "/"))
	var out []cmdInfo
	for _, c := range commands {
		name := strings.ToLower(strings.TrimPrefix(c.name, "/"))
		if strings.HasPrefix(name, typed) {
			out = append(out, c)
		}
	}
	return out
}

// applyCommand resolves the highlighted autocomplete item.
func (m *model) applyCommand(item cmdInfo) {
	switch item.name {
	case "/model":
		m.input = ""
		m.openPicker()
	case "/key":
		m.input = "/key "
	default:
		m.input = ""
		m.handleCommand(item.name)
	}
	m.autoIdx = 0
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
		if sel == "local" {
			// Auto-discover local models on this machine.
			m.pickLocal = true
			m.pickOnes = m.pickTwo["local"]
			m.pickIdx = 0
			m.picker = true
			return m.discoverLocalCmd()
		}
		m.pickLocal = false
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
	if m.cfg.ActiveProvider == "local" {
		if base, ok := m.localMap[sel]; ok && base != "" {
			m.cfg.Providers["local"].BaseURL = base
		}
	}
	m.cfg.Active().Model = sel
	m.cfg.Save()
	m.picker = false
	m.pickLocal = false
	m.entries = append(m.entries, entry{kind: "system", text: fmt.Sprintf("switched to %s (%s)", m.cfg.ActiveProvider, sel)})
	return nil
}

// discoverLocalCmd scans the machine for local models in the background.
func (m *model) discoverLocalCmd() tea.Cmd {
	base := ""
	if p, ok := m.cfg.Providers["local"]; ok {
		base = p.BaseURL
	}
	return func() tea.Msg {
		return localModelsMsg{models: provider.DiscoverLocalModels(base)}
	}
}

func (m *model) submit(line string) tea.Cmd {
	// Record the prompt in history for ↑/↓ recall.
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(line, "/") {
		return m.handleCommand(line)
	}
	if trimmed != "" && (len(m.history) == 0 || m.history[len(m.history)-1] != trimmed) {
		m.history = append(m.history, trimmed)
	}
	m.histIdx = len(m.history)

	m.entries = append(m.entries, entry{kind: "user", text: line})
	m.busy = true
	m.scroll = 0 // pin to the latest output when a new turn starts
	// Pass plan/build mode to the agent before this turn starts.
	m.ag.Plan = m.plan
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	m.ctx, m.cancel = ctx, cancel

	// Stream events to the UI in real time: tool calls, tool results,
	// and the final answer appear as they happen, not all at once.
	ch := make(chan turnEventMsg, 128)
	m.streamCh = ch

	// Permission gate: before the agent touches anything, ask the user.
	// The callback blocks (in the agent goroutine) until the UI answers.
	m.ag.Confirm = func(req agent.ConfirmRequest) bool {
		select {
		case ch <- turnEventMsg{ev: agent.TurnEvent{Type: "confirm", Tool: req.Tool, Text: req.Args}}:
		case <-ctx.Done():
			return false
		}
		reply := make(chan bool, 1)
		m.confirmReply = reply
		select {
		case ok := <-reply:
			return ok
		case <-ctx.Done():
			return false
		}
	}

	go func() {
		defer close(ch)
		defer cancel()
		content, err := m.ag.Turn(ctx, line, func(ev agent.TurnEvent) {
			select {
			case ch <- turnEventMsg{ev: ev}:
			case <-ctx.Done():
			}
		})
		if err == nil && strings.TrimSpace(content) != "" {
			select {
			case ch <- turnEventMsg{ev: agent.TurnEvent{Type: "answer", Text: content}}:
			case <-ctx.Done():
			}
		}
		select {
		case ch <- turnEventMsg{done: true, err: err}:
		case <-ctx.Done():
		}
	}()

	return m.nextStream()
}

// nextStream returns a cmd that reads one event from the stream channel.
// Update() re-invokes it after each event so events keep flowing.
func (m *model) nextStream() tea.Cmd {
	ch := m.streamCh
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return turnEventMsg{done: true}
		}
		return ev
	}
}

func (m *model) pushEvent(ev agent.TurnEvent) {
	switch ev.Type {
	case "tool":
		m.entries = append(m.entries, entry{kind: "tool", text: "fins up → " + ev.Tool + " " + truncate(ev.Args, 80)})
	case "tool_result":
		m.entries = append(m.entries, entry{kind: "tool_result", text: truncate(ev.Result, 300)})
	case "token":
		// Live streamed reply text: accumulate into the current assistant entry.
		if !m.streaming {
			m.streaming = true
			m.entries = append(m.entries, entry{kind: "assistant", text: ev.Text})
		} else if len(m.entries) > 0 {
			last := &m.entries[len(m.entries)-1]
			last.text += ev.Text
		}
	case "answer":
		m.streaming = false
		// If tokens already streamed into the last entry, replace it with the
		// complete answer instead of duplicating.
		if len(m.entries) > 0 {
			last := &m.entries[len(m.entries)-1]
			if last.kind == "assistant" {
				last.text = ev.Text
				return
			}
		}
		m.entries = append(m.entries, entry{kind: "assistant", text: ev.Text})
	case "error":
		m.entries = append(m.entries, entry{kind: "error", text: ev.Text})
	case "confirm":
		m.confirmTool = ev.Tool
		m.confirmArgs = ev.Text
		m.entries = append(m.entries, entry{kind: "confirm", text: ev.Tool + " " + ev.Text})
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

	case "/verify":
		m.ag.Verify = !m.ag.Verify
		state := "off"
		if m.ag.Verify {
			state = "on"
		}
		m.entries = append(m.entries, entry{kind: "system", text: "accuracy gate " + state})

	case "/plan":
		m.plan = true
		m.entries = append(m.entries, entry{kind: "system", text: "switched to plan mode (read-only) — tab or /build to exit"})

	case "/build":
		m.plan = false
		m.entries = append(m.entries, entry{kind: "system", text: "switched to build mode — tab or /plan to change"})

	case "/clear":
		m.entries = nil
		m.ag.Clear()

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
  /verify                toggle the accuracy gate (on/off)
  /plan / /build         switch modes (or press Tab)
  /providers             list providers and their models
  /clear                 clear conversation
  /help                  this help
  /exit                  quit

slash autocomplete:
  type "/" then the start of a command (e.g. /mod) and press Enter
  to see live suggestions, navigate with ↑/↓, Enter to pick.

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

// displayUserText renders what the user sent in the chat log. Long pasted
// or typed input collapses into a compact "pasted N lines" summary so the
// conversation stays readable (opencode-style).
func displayUserText(s string) string {
	lines := strings.Split(s, "\n")
	// Collapse a single whitespace-only line to keep the log tidy.
	if len(lines) <= 1 && strings.TrimSpace(s) == "" {
		return s
	}
	if len(lines) > 4 || len(s) > 300 {
		// show the first line plus a collapse marker
		first := strings.TrimSpace(lines[0])
		if len(first) > 80 {
			first = first[:80] + "…"
		}
		return fmt.Sprintf("pasted %d lines · %d chars\n%s", len(lines), len(s), first)
	}
	return s
}

func center(s string, width int) string {
	if width <= 0 {
		return s
	}
	var b strings.Builder
	for i, l := range strings.Split(s, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		w := lipgloss.Width(l)
		if w >= width {
			b.WriteString(l)
			continue
		}
		b.WriteString(strings.Repeat(" ", (width-w)/2))
		b.WriteString(l)
	}
	return b.String()
}

// hasChat reports whether a real conversation has started.
func (m *model) hasChat() bool {
	for _, e := range m.entries {
		switch e.kind {
		case "user", "assistant", "tool", "tool_result", "error":
			return true
		}
	}
	return false
}

func (m *model) View() string {
	if m.water == nil {
		m.water = NewWater(80, 24)
	}
	bg := m.water.Render()

	welcome := !m.picker && !m.busy && !m.hasChat()

	var overlay []string
	if welcome {
		overlay = m.welcomeLines()
	}

	if welcome {
		start := (len(bg) - len(overlay)) / 2
		if start < 0 {
			start = 0
		}
		for i, line := range overlay {
			if start+i < len(bg) {
				bg[start+i] = m.seaLine(line, start+i)
			}
		}
	} else {
		header, body, footer := m.chatLines()

		// Layout: header pinned at top, status bar pinned at the very
		// bottom, input footer anchored just above it, content scrolling
		// in the space between.
		footerStart := m.height - len(footer) - 1 // reserve status bar row
		if footerStart < 1 {
			footerStart = 1
		}
		contentViewport := footerStart - 1
		if contentViewport < 1 {
			contentViewport = 1
		}
		if len(body) > contentViewport {
			maxScroll := len(body) - contentViewport
			if m.scroll > maxScroll {
				m.scroll = maxScroll
			}
			start := maxScroll - m.scroll
			body = body[start : start+contentViewport]
		} else {
			m.scroll = 0
		}

		// Build the right info panel once per frame.
		panel := m.panelContent(len(bg))

		// Draw header pinned at the top.
		bg[0] = m.renderRow(header, 0, panel)
		// Draw scrollable content.
		row := 1
		for _, line := range body {
			if row < footerStart && row < len(bg) {
				bg[row] = m.renderRow(line, row, panel)
			}
			row++
		}
		// Anchor the input footer at the bottom, above the status bar.
		row = footerStart
		for _, line := range footer {
			if row < len(bg) {
				bg[row] = m.renderRow(line, row, panel)
			}
			row++
		}
		// Full-width opencode-style status bar as the last row.
		if statusRow := m.height - 1; statusRow >= 0 && statusRow < len(bg) {
			bg[statusRow] = m.statusBar()
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

	// Render the permission modal on top of the chat (opencode-style popup).
	if m.confirmReply != nil {
		modal := m.confirmPrompt()
		modalLines := strings.Split(modal, "\n")
		start := (len(bg) - len(modalLines)) / 2
		if start < 0 {
			start = 0
		}
		for i, ml := range modalLines {
			row := start + i
			if row >= 0 && row < len(bg) {
				bg[row] = ml
			}
		}
	}

	return strings.Join(bg, "\n")
}

// seaLine renders one overlay line on the sea background: the text itself
// sits on the water color, and any leftover row width is filled by the
// underwater backdrop (bubbles) so they float BEHIND the text.
func (m *model) seaLine(line string, y int) string {
	return m.seaTo(line, y, m.width)
}

// renderRow lays out one terminal row with the sea behind it: the chat
// content occupies the left band (up to chatWidth) and the info panel the
// right. panel is the pre-rendered slice from panelContent.
func (m *model) renderRow(line string, y int, panel []string) string {
	side := m.sidebar()
	if side <= 0 {
		return m.seaLine(line, y)
	}
	left := m.seaTo(line, y, m.chatWidth())
	right := ""
	if y >= 0 && y < len(panel) {
		right = panel[y]
	} else {
		right = m.water.Seg(y, m.chatWidth(), m.width)
	}
	return left + right
}

// seaTo renders text on the sea background with the sea filling the band
// from the end of the text to column width. Used by the row composer so the
// chat band stops before the right info panel starts.
func (m *model) seaTo(line string, y, width int) string {
	tw := lipgloss.Width(line)
	if tw < 0 {
		tw = 0
	}
	col := m.water.bgFor(y)

	var styled string
	if tw >= width {
		styled = lipgloss.NewStyle().Background(col).Render(line)
	} else {
		styled = lipgloss.NewStyle().Background(col).Render(line) + m.water.Seg(y, tw, width)
	}

	// Re-assert the sea background after every ANSI reset. Multi-line
	// styled blobs (error JSON, tool results, help text) contain interior
	// "\x1b[0m" sequences that otherwise clear the background back to the
	// terminal default (black), leaving black patches behind the letters.
	seq := lipgloss.NewStyle().Background(col).Render("A")
	if i := strings.Index(seq, "A"); i >= 0 {
		seq = seq[:i]
		styled = strings.ReplaceAll(styled, "\x1b[0m", "\x1b[0m"+seq)
		styled = strings.ReplaceAll(styled, "\x1b[m", "\x1b[m"+seq)
	}
	return styled
}

// welcomeLines builds the centered landing view (opencode-style).
func (m *model) welcomeLines() []string {
	p := m.cfg.Active()
	modelName := "none"
	if p != nil {
		modelName = p.Model
	}

	input := PromptStyle.Render("> ") + BodyStyle.Render(m.input+"▌")

	title := TitleStyle.Render("SHARKCODE")
	tagline := TaglineStyle.Render(m.tagline())

	cmdline := HintStyle.Render("/model · /key · /providers · /clear · /help · /exit")
	status := HintStyle.Render(fmt.Sprintf("currently: %s (%s)", m.cfg.ActiveProvider, modelName))

	var lines []string
	lines = append(lines, "")
	lines = append(lines, center(SharkLogo(), m.width))
	lines = append(lines, "")
	lines = append(lines, center(title, m.width))
	lines = append(lines, center(tagline, m.width))
	lines = append(lines, "")
	lines = append(lines, center(input, m.width))
	lines = append(lines, center(cmdline, m.width))
	lines = append(lines, center(HintStyle.Render(m.contextLine()), m.width))
	lines = append(lines, center(status, m.width))

	if matches := m.menuMatches(); len(matches) > 0 {
		lines = append(lines, "")
		for _, ml := range strings.Split(m.renderMenu(matches), "\n") {
			lines = append(lines, center(ml, m.width))
		}
	}
	return lines
}

// modeName returns the current plan/build mode label.
func (m *model) modeName() string {
	if m.plan {
		return "plan"
	}
	return "build"
}

// tagline returns a rotating shark pun for the welcome screen.
func (m *model) tagline() string {
	taglines := []string{
		"the terminal coding agent",
		"your code's apex predator",
		"no fluff, just chomp",
		"faster than your average fish",
		"swimming through your stack",
		"fins up, bugs down",
		"a bite of pure productivity",
		"shark-mode: engaged",
	}
	return taglines[m.spinner%len(taglines)]
}

// contextLine renders the opencode-style context indicator:
// tokens used · % of window · cost.
func (m *model) contextLine() string {
	u := m.ag.Usage
	total := u.Total()
	win := m.ag.ContextWindow()
	used := 0
	if win > 0 {
		used = total * 100 / win
	}
	gate := "off"
	if m.ag.Verify {
		gate = "on"
	}
	return fmt.Sprintf("[%s] context: %s tokens · %d%% used · $%.2f · gate %s",
		m.modeName(), formatInt(total), used, u.CostUSD, gate)
}

// statusBar returns the full-width bottom status bar: usage on the left,
// model/mode/gate on the right — opencode-style.
func (m *model) statusBar() string {
	u := m.ag.Usage
	total := u.Total()
	win := 0
	if m.reg != nil {
		win = m.ag.ContextWindow()
	}
	used := 0
	if win > 0 {
		used = total * 100 / win
	}
	gate := "off"
	if m.ag.Verify {
		gate = "on"
	}
	p := m.cfg.Active()
	modelName := "none"
	if p != nil {
		modelName = p.Model
	}

	left := fmt.Sprintf("🦈 shark · %s · %s tokens · %d%% context · $%.2f",
		m.modeName(), formatInt(total), used, u.CostUSD)
	right := fmt.Sprintf("gate %s · %s", gate, modelName)

	filler := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if filler < 0 {
		filler = 0
	}
	space := lipgloss.NewStyle().Background(DeepBG).Render(strings.Repeat(" ", filler))
	return StatusBar.Render(" ") + StatusBar.Render(left) + space + StatusBar.Render(right) + " "
}

func formatInt(n int) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// chatLines builds the conversation view, splitting it into a pinned
// header, a scrollable body, and a pinned footer (input/hint/context/menu).
func (m *model) chatLines() (header string, body, footer []string) {
	p := m.cfg.Active()
	modelName := "none"
	if p != nil {
		modelName = p.Model
	}
	status := fmt.Sprintf("  🦈 %s · %s · [%s]", m.cfg.ActiveProvider, modelName, m.modeName())
	if m.busy {
		frames := []string{"sharking", "chompin'", "swimming", "hunting bugs", "sniffin' water", "fin-twitching", "glubbing"}
		status = fmt.Sprintf("  🦈 %s · %s · %s · [%s]", m.cfg.ActiveProvider, frames[m.spinner%len(frames)], modelName, m.modeName())
	}

	var parts []string
	for _, e := range m.entries {
		switch e.kind {
		case "user":
			parts = append(parts, UserChip.Render("you 🐟")+"\n"+BodyStyle.Render(displayUserText(e.text)))
		case "assistant":
			rich := renderRich(e.text)
			body := strings.Join(rich, "\n")
			parts = append(parts, SharkChip.Render("🦈 shark")+"\n"+body)
		case "tool":
			parts = append(parts, HintStyle.Render("  · " + e.text))
		case "tool_result":
			parts = append(parts, HintStyle.Render("  · " + e.text))
		case "error":
			parts = append(parts, ErrorStyle.Render("⚠ " + e.text))
		case "system":
			parts = append(parts, HintStyle.Render("· " + e.text))
		case "confirm":
			parts = append(parts, CoralLabel.Render("permission needed → " + e.text))
		case "help":
			rich := renderRich(e.text)
			parts = append(parts, strings.Join(rich, "\n"))
		}
	}
	content := strings.Join(parts, "\n"+WaveDivider+"\n")

	// Live confirmation prompt while the agent waits for approval.
	// A multi-line input (e.g. a big paste) is shown collapsed as
	// "pasted N lines" so the prompt stays a single crisp line.
	inputText := m.input
	if strings.Contains(m.input, "\n") {
		n := strings.Count(m.input, "\n") + 1
		first := strings.TrimSpace(strings.Split(m.input, "\n")[0])
		if len(first) > 60 {
			first = first[:60] + "…"
		}
		if first != "" {
			inputText = fmt.Sprintf("\x1b[0mpasted %d lines · %s", n, first)
		} else {
			inputText = fmt.Sprintf("\x1b[0mpasted %d lines", n)
		}
	}
	input := PromptStyle.Render("> ") + BodyStyle.Render(inputText+"▌")
	hint := HintStyle.Render("type / to see commands · tab = plan/build · esc esc = cancel")

	overlay := []string{HeaderStyle.Width(m.width).Render(status)}
	pad := strings.Repeat(" ", 2)
	padLen := lipgloss.Width(pad)
	wrapW := m.chatWidth() - padLen - 2 // 2-space breathing room on the right
	if wrapW < 20 {
		wrapW = 20
	}
	contentLines := strings.Split(content, "\n")
	for _, cl := range contentLines {
		if strings.TrimSpace(cl) == "" {
			overlay = append(overlay, "")
			continue
		}
		for _, wl := range strings.Split(ansi.Wrap(cl, wrapW, " \t/\\"), "\n") {
			overlay = append(overlay, pad+wl)
		}
	}
	body = overlay[1:]
	footer = append(footer, "", input, hint)

	if matches := m.menuMatches(); len(matches) > 0 {
		footer = append(footer, "")
		for _, ml := range strings.Split(m.renderMenu(matches), "\n") {
			footer = append(footer, ml)
		}
	}
	return overlay[0], body, footer
}

// confirmPrompt renders the permission request as a centered modal box
// (opencode-style popup) that drops over the still-visible chat log.
func (m *model) confirmPrompt() string {
	tool := m.confirmTool
	if tool == "" {
		tool = "this action"
	}
	args := truncate(m.confirmArgs, 200)

	var b strings.Builder
	b.WriteString(TitleStyle.Render("🦈 permission required") + "\n")
	b.WriteString(TaglineStyle.Render("the shark wants to ") + CoralLabel.Render(tool) + "\n")
	if args != "" {
		b.WriteString(BodyStyle.Render(truncate(args, 160)) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(MenuSelStyle.Render(" y ") + HintStyle.Render(" allow") +
		"    " + ErrorStyle.Render(" n ") + HintStyle.Render(" deny") + "\n")
	b.WriteString(HintStyle.Render("y / enter = allow   n / esc = deny"))

	return center(
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Coral).
			Background(DeepBG).
			Padding(1, 2).
			Render(b.String()),
		m.width,
	)
}

// renderMenu renders the slash-command suggestion popup.
func (m *model) renderMenu(matches []cmdInfo) string {
	if len(matches) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(HintStyle.Render("type to filter · ↑/↓ navigate · enter select · esc close") + "\n")
	for i, c := range matches {
		marker := "  "
		line := c.name + "   " + HintStyle.Render(c.desc)
		style := BodyStyle
		if i == m.autoIdx {
			marker = "▶ "
			style = MenuSelStyle
		}
		b.WriteString(style.Render(marker+line) + "\n")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Cyan).
		Background(DeepBG).
		Padding(0, 2).
		Render(strings.TrimSuffix(b.String(), "\n"))
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
			style := BodyStyle
			if i == m.pickIdx {
				marker = "▶ "
				style = MenuSelStyle
			}
			b.WriteString(style.Render(marker + label) + "\n")
		}
	} else {
		for i, model := range m.pickOnes {
			marker := "  "
			style := BodyStyle
			if i == m.pickIdx {
				marker = "▶ "
				style = MenuSelStyle
			}
			b.WriteString(style.Render(marker+model) + "\n")
		}
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Teal).
		Background(DeepBG).
		Padding(0, 2).
		Render(b.String())
	return box
}
