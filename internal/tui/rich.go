package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderRich converts assistant/help text containing light markdown into
// styled output: it extracts fenced ``` code blocks into dark panels,
// styles headers and list bullets, and leaves the rest as body text.
// Returns the text pre-split so chatLines can keep the identifier line
// (e.g. "shark") as its own first line.
func renderRich(text string) []string {
	var out []string
	var buf []string // plain line accumulator (outside a code block)
	var codeLang string
	inCode := false

	flush := func() {
		if len(buf) == 0 {
			return
		}
		for _, ln := range buf {
			out = append(out, renderLine(ln))
		}
		buf = nil
	}

	for _, raw := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(raw)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			flush()
			if inCode {
				out = append(out, renderCodePanel(codeLang, buf))
				buf = nil
				inCode = false
				codeLang = ""
			} else {
				inCode = true
				codeLang = strings.Trim(strings.Join(strings.Fields(trim)[1:], " "), " ")
			}
			continue
		}
		buf = append(buf, raw)
	}
	if inCode {
		out = append(out, renderCodePanel(codeLang, buf))
	} else {
		flush()
	}
	return out
}

// renderLine styles a single markdown-ish line: headings, rules, quotes,
// list bullets, and inline code.
func renderLine(l string) string {
	trimmed := strings.TrimSpace(l)

	// Blockquote "> note"
	if strings.HasPrefix(trimmed, "> ") {
		return QuoteBar + renderInline(strings.TrimSpace(trimmed[2:]))
	}
	// Headings (#, ##, ###)
	if strings.HasPrefix(trimmed, "### ") {
		return lipgloss.NewStyle().Foreground(Cyan).Bold(true).Underline(true).Render(strings.TrimPrefix(trimmed, "### "))
	}
	if strings.HasPrefix(trimmed, "## ") {
		return SharkLabel.Render("🌊  " + strings.TrimPrefix(trimmed, "## "))
	}
	if strings.HasPrefix(trimmed, "# ") {
		return TitleStyle.Render(strings.TrimPrefix(trimmed, "# "))
	}
	// horizontal rule
	if trimmed == "---" || trimmed == "***" || trimmed == "___" {
		return HintStyle.Render(strings.Repeat("─", 40))
	}
	// unordered list
	if len(trimmed) > 1 && (strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") || strings.HasPrefix(trimmed, "• ")) {
		return ListBullet + renderInline(strings.TrimSpace(trimmed[2:]))
	}
	// ordered list "1. " / "1) "
	if len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' {
		if j := indexAfterNum(trimmed); j > 0 {
			num := trimmed[:j]
			rest := strings.TrimSpace(trimmed[j+1:])
			return lipgloss.NewStyle().Foreground(Cyan).Bold(true).Render(num+".") + " " + renderInline(rest)
		}
	}
	return BodyStyle.Render(renderInline(trimmed))
}

// indexAfterNum returns the index just past a leading "123." or "123)"
// sequence (the ". " or ") " separator), or 0 if the line doesn't start
// with a numbered list marker.
func indexAfterNum(s string) int {
	j := 0
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	if j == 0 || j >= len(s) {
		return 0
	}
	if s[j] != '.' && s[j] != ')' {
		return 0
	}
	if j+1 >= len(s) || s[j+1] != ' ' {
		return 0
	}
	return j
}

// renderInline styles backtick spans as inline code.
func renderInline(s string) string {
	if !strings.Contains(s, "`") {
		return BodyStyle.Render(s)
	}
	var b strings.Builder
	for i, token := range strings.Split(s, "`") {
		if i%2 == 1 {
			b.WriteString(InlineCodeStyle.Render(token))
		} else {
			b.WriteString(BodyStyle.Render(token))
		}
	}
	return b.String()
}

// renderCodePanel wraps code lines in a bordered dark panel (opencode-style)
// with a small language chip in the top-left corner.
func renderCodePanel(lang string, lines []string) string {
	body := strings.Join(lines, "\n")
	var header string
	if lang != "" {
		header = LangChip.Render(lang) + "\n"
	}
	inner := header + lipgloss.NewStyle().Background(CodeBG).Foreground(Mono).Render(body)
	return CodeBlockStyle.Render(inner)
}

// renderCode is a compatibility shim kept for clarity.
func renderCode(lang string, lines []string) string {
	return renderCodePanel(lang, lines)
}