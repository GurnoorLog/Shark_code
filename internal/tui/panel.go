package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// sidebar returns the column width of the right info panel. It shrinks on
// narrow terminals and disappears entirely below ~78 columns so the chat
// band never gets crushed.
func (m *model) sidebar() int {
	switch {
	case m.width >= 150:
		return 36
	case m.width >= 116:
		return 30
	case m.width >= 88:
		return 24
	default:
		return 0
	}
}

// chatWidth returns the column width available to the conversation band
// (everything left of the sidebar).
func (m *model) chatWidth() int {
	return m.width - m.sidebar()
}

// panelContent builds the right info panel: context usage, model, mode,
// gate and a couple of cheeky shark lines (opencode-style side panel).
// It returns one styled line per visible chat row, with empty trailing rows.
func (m *model) panelContent(h int) []string {
	side := m.sidebar()
	if side <= 0 || h <= 0 {
		return nil
	}

	u := m.ag.Usage
	total := u.Total()
	win := 0
	if m.reg != nil && m.ag != nil {
		win = m.ag.ContextWindow()
	}
	used := 0
	if win > 0 {
		used = total * 100 / win
		if used > 100 {
			used = 100
		}
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

	var lines []string
	title := " " + SharkChip.Render("🦈 shark")
	lines = append(lines, lipgloss.NewStyle().Background(PanelBG).Render(title))

	label := func(s string) string {
		return " " + lipgloss.NewStyle().
			Foreground(Bubble).
			Bold(true).
			Background(PanelBG).
			Render(s)
	}
	val := func(s string) string {
		return "  " + lipgloss.NewStyle().Foreground(White).Background(PanelBG).Render(s)
	}
	dim := func(s string) string {
		return "  " + lipgloss.NewStyle().Foreground(Bubble).Background(PanelBG).Render(s)
	}

	// progress bar for context usage, opencode-style.
	barW := side - 6
	if barW < 4 {
		barW = 4
	}
	fill := barW * used / 100
	bar := strings.Repeat("█", fill) + strings.Repeat("░", barW-fill)

	lines = append(lines, label("context"))
	lines = append(lines, val(fmt.Sprintf("%s tokens", formatInt(total))))
	lines = append(lines, val(fmt.Sprintf("%d%% used", used)))
	lines = append(lines, dim("$"+fmt.Sprintf("%.2f", u.CostUSD)+" spent"))
	lines = append(lines, " "+lipgloss.NewStyle().Foreground(Teal).Background(PanelBG).Render(bar))

	lines = append(lines, label("model"))
	lines = append(lines, val(m.cfg.ActiveProvider))
	lines = append(lines, val(modelName))

	lines = append(lines, label("mode"))
	lines = append(lines, val(m.modeName()))
	lines = append(lines, val("gate "+gate))

	// pad every line to the full panel width so the sidebar reads as a
	// solid slab, and blank-away any overflow.
	out := make([]string, 0, h)
	bg := lipgloss.NewStyle().Background(PanelBG).Render(" ")
	for i := 0; i < h; i++ {
		if i < len(lines) {
			l := lines[i]
			w := lipgloss.Width(l)
			if w < side {
				l += strings.Repeat(" ", side-w)
			}
			out = append(out, l)
		} else {
			out = append(out, strings.Repeat(bg, side))
		}
	}
	return out
}