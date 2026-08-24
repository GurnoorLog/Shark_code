package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// sidebar returns the column width of the right info panel band. It
// shrinks on narrow terminals and disappears entirely below ~78 columns
// so the chat band never gets crushed.
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

// panelContent builds the right info box: context usage, model, mode and
// gate inside a rounded border that FLOATS on the water. There is no solid
// slab behind it: rows without content come back as "" and renderRow fills
// them with live water, so bubbles rise behind and around the box.
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

	// Everything inside the box must fit within side-4 columns (border +
	// padding), or the box would grow into the chat band.
	innerMax := side - 4 - 2 // border(2) + padding(2)
	if innerMax < 8 {
		innerMax = 8
	}
	fit := func(s string) string { return ansiTrunc(s, innerMax) }

	barW := innerMax
	fill := barW * used / 100
	track := lipgloss.NewStyle().Foreground(lipgloss.Color("#16405e"))
	bar := PanelBar.Render(strings.Repeat("█", fill)) + track.Render(strings.Repeat("░", barW-fill))

	var lines []string
	lines = append(lines, fit(SharkChip.Render(" 🦈 shark ")))
	lines = append(lines, "")
	lines = append(lines, PanelLabel.Render("context"))
	lines = append(lines, PanelValue.Render(fmt.Sprintf("%s tok · %d%%", formatInt(total), used)))
	lines = append(lines, bar)
	lines = append(lines, PanelValueDim.Render(fmt.Sprintf("$%.2f spent", u.CostUSD)))
	lines = append(lines, "")
	lines = append(lines, PanelLabel.Render("model"))
	lines = append(lines, PanelValue.Render(fit(modelName)))
	lines = append(lines, "")
	lines = append(lines, PanelLabel.Render("mode"))
	lines = append(lines, PanelValue.Render(m.modeName()+" · gate "+gate))

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#1a5f7a")).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))

	out := make([]string, h)
	for i, l := range strings.Split(box, "\n") {
		if i < h {
			out[i] = l
		}
	}
	return out
}

// ansiTrunc truncates a styled string to n cells.
func ansiTrunc(s string, n int) string {
	if lipgloss.Width(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, "…")
}
