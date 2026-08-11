package tui

import (
	"strings"
	"testing"
)

func TestRenderRich(t *testing.T) {
	in := "Step one\n\n1. First item\n- Bullet item\n\n```go\npackage main\n\nfunc main() {}\n```\n\nDone."
	lines := renderRich(in)

	var joined strings.Builder
	for _, l := range lines {
		joined.WriteString(l + "\n")
	}
	full := joined.String()

	if !strings.Contains(full, "1.") {
		t.Errorf("expected ordered-list number, got:\n%s", full)
	}
	if !strings.Contains(full, "▸") {
		t.Errorf("expected unordered bullet, got:\n%s", full)
	}
	if !strings.Contains(full, "package") {
		t.Errorf("expected code body present, got:\n%s", full)
	}
	if !strings.Contains(full, "func main") {
		t.Errorf("expected code body present, got:\n%s", full)
	}
}

func TestRenderRichInlineCode(t *testing.T) {
	got := renderInline("use `Go` here")
	if !strings.Contains(got, "Go") {
		t.Errorf("expected inline code content, got: %q", got)
	}
}