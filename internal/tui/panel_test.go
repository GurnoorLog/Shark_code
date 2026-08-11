package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"shark-agent/internal/agent"
	"shark-agent/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		ActiveProvider: "openai",
		Providers: map[string]*config.ProviderConfig{
			"openai": {Model: "gpt-4o"},
		},
	}
}

func TestPanelContentRows(t *testing.T) {
	m := &model{width: 130, height: 40}
	m.cfg = testConfig()
	m.ag = &agent.Agent{Usage: agent.Usage{InputTokens: 5000, OutputTokens: 2000}, Verify: true}
	m.water = NewWater(130, 40)

	panel := m.panelContent(40)
	if len(panel) != 40 {
		t.Fatalf("panel rows = %d, want 40", len(panel))
	}
	for i, row := range panel {
		if lipgloss.Width(row) < m.sidebar() {
			t.Errorf("panel row %d width %d < sidebar %d", i, lipgloss.Width(row), m.sidebar())
		}
	}
}

func TestStatusBar(t *testing.T) {
	m := &model{width: 120, height: 30}
	m.cfg = testConfig()
	m.ag = &agent.Agent{Usage: agent.Usage{InputTokens: 5000}, Verify: true}

	sb := m.statusBar()
	if !strings.Contains(sb, "context") {
		t.Fatalf("status bar missing usage: %q", sb)
	}
	if !strings.Contains(sb, "gate") {
		t.Fatalf("status bar missing gate: %q", sb)
	}
	if lipgloss.Width(sb) != 120 {
		t.Fatalf("status bar width = %d, want 120", lipgloss.Width(sb))
	}
}

func TestRenderRowWidths(t *testing.T) {
	m := &model{width: 120, height: 30}
	m.cfg = testConfig()
	m.ag = &agent.Agent{}
	m.water = NewWater(120, 30)
	panel := m.panelContent(30)
	row := m.renderRow("hello", 5, panel)
	if lipgloss.Width(row) != 120 {
		t.Fatalf("row width = %d, want 120", lipgloss.Width(row))
	}
}

func TestSidebarHidesNarrow(t *testing.T) {
	m := &model{width: 60, height: 20}
	if m.sidebar() != 0 {
		t.Fatalf("sidebar should hide on narrow terminals, got %d", m.sidebar())
	}
	if m.chatWidth() != 60 {
		t.Fatalf("chatWidth = %d, want 60", m.chatWidth())
	}
}