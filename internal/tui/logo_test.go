package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestSharkLogoAlignment(t *testing.T) {
	art := SharkLogo()
	rows := strings.Split(art, "\n")
	width := lipgloss.Width(art)
	if width == 0 {
		t.Fatal("logo rendered empty")
	}
	t.Logf("logo width: %d, rows: %d", width, len(rows))
	// Strip ANSI to inspect the raw art.
	var plain []string
	for _, r := range rows {
		plain = append(plain, ansiRe2.ReplaceAllString(r, ""))
	}
	t.Logf("ART:\n%s", strings.Join(plain, "\n"))
}