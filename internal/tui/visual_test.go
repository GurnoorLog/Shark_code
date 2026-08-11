package tui

import (
	"regexp"
	"strings"
	"testing"

	"shark-agent/internal/agent"
	"shark-agent/internal/provider"
)

var ansiRe2 = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func TestWelcomeFrame(t *testing.T) {
	m := &model{width: 120, height: 46}
	m.cfg = testConfig()
	m.reg = provider.NewRegistry(testConfig())
	m.ag = &agent.Agent{}
	m.water = NewWater(120, 46)

	out := m.View()
	plain := ansiRe2.ReplaceAllString(out, "")
	if !strings.Contains(plain, "SHARKCODE") {
		t.Fatalf("missing title:\n%s", plain)
	}
	if !strings.Contains(plain, "⣿") {
		t.Fatalf("braille shark missing from welcome:\n%s", plain)
	}
	t.Logf("WELCOME FRAME:\n%s", plain)
}