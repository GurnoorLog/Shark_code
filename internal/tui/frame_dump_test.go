package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"shark-agent/internal/agent"
	"shark-agent/internal/provider"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

func newTestModel(w, h int) *model {
	m := &model{width: w, height: h}
	m.cfg = testConfig()
	m.reg = provider.NewRegistry(testConfig())
	m.ag = &agent.Agent{}
	m.water = NewWater(w, h)
	return m
}

func TestWelcomeTiers(t *testing.T) {
	cases := []struct {
		w, h    int
		wantArt string // substring expected from the art tier
	}{
		{120, 46, "⣿"},        // full braille shark
		{100, 30, ">≈((((º>"}, // compact mark
		{80, 24, ">≈((((º>"},  // still room for the mark
	}
	for _, c := range cases {
		m := newTestModel(c.w, c.h)
		out := stripANSI(m.View())
		if !strings.Contains(out, c.wantArt) {
			t.Errorf("%dx%d: expected art %q in welcome frame", c.w, c.h, c.wantArt)
		}
		for _, want := range []string{"SHARKCODE", "describe a task", "/help"} {
			if !strings.Contains(out, want) {
				t.Errorf("%dx%d: welcome missing %q", c.w, c.h, want)
			}
		}
	}
}

func TestNoBlackEdges(t *testing.T) {
	for _, sz := range [][2]int{{120, 46}, {100, 30}, {80, 24}} {
		m := newTestModel(sz[0], sz[1])
		rows := strings.Split(m.View(), "\n")
		if len(rows) != sz[1] {
			t.Errorf("%dx%d: frame has %d rows", sz[0], sz[1], len(rows))
		}
		for i, r := range rows {
			if got := lipgloss.Width(r); got != sz[0] {
				t.Errorf("%dx%d row %d: width %d != %d", sz[0], sz[1], i, got, sz[0])
			}
		}
	}
}

func TestChatFrameLayout(t *testing.T) {
	m := newTestModel(110, 32)
	m.entries = append(m.entries,
		entry{kind: "user", text: "hello shark"},
		entry{kind: "assistant", text: "fin-tastic"},
	)
	rows := strings.Split(m.View(), "\n")
	if len(rows) != 32 {
		t.Fatalf("chat frame has %d rows, want 32", len(rows))
	}
	for i, r := range rows {
		if got := lipgloss.Width(r); got != 110 {
			t.Errorf("row %d width %d != 110", i, got)
		}
	}
}

func TestOverlayFramesFullWidth(t *testing.T) {
	t.Run("picker", func(t *testing.T) {
		m := newTestModel(100, 30)
		m.openPicker()
		assertFullFrame(t, m, 100, 30)
	})
	t.Run("confirm modal", func(t *testing.T) {
		m := newTestModel(100, 30)
		m.entries = append(m.entries, entry{kind: "user", text: "run it"})
		m.confirmTool = "bash"
		m.confirmArgs = "dir"
		ch := make(chan bool, 1)
		ch <- true
		m.confirmReply = ch
		assertFullFrame(t, m, 100, 30)
	})
}

func TestStatusBarNarrow(t *testing.T) {
	m := newTestModel(60, 20)
	m.cfg.Providers["openai"].Model = "accounts/fireworks/models/kimi-k2p7-code-rc-extra-long"
	sb := m.statusBar()
	if got := lipgloss.Width(sb); got != 60 {
		t.Fatalf("narrow status bar width = %d, want 60", got)
	}
}

func TestPanelBoxFloats(t *testing.T) {
	m := newTestModel(120, 28)
	m.cfg.Providers["openai"].Model = "gemini-2.5-flash-a-very-long-model-name-indeed"
	m.entries = append(m.entries, entry{kind: "user", text: "hi"})
	rows := strings.Split(m.View(), "\n")

	if testing.Verbose() {
		for _, r := range rows {
			t.Logf("|%s|", stripANSI(r))
		}
	}

	boxRows := 0
	for _, r := range rows {
		plain := stripANSI(r)
		if strings.Contains(plain, "╭") || strings.Contains(plain, "│") || strings.Contains(plain, "╰") {
			boxRows++
		}
	}
	if boxRows < 10 {
		t.Fatalf("expected a bordered info box, saw %d border rows", boxRows)
	}

	panel := m.panelContent(28)
	for i, p := range panel {
		if p != "" && lipgloss.Width(p) > m.sidebar() {
			t.Errorf("panel row %d overflows sidebar: %d > %d", i, lipgloss.Width(p), m.sidebar())
		}
	}
	for i, r := range rows {
		if got := lipgloss.Width(r); got != 120 {
			t.Errorf("row %d width %d != 120", i, got)
		}
	}
}

func assertFullFrame(t *testing.T, m *model, w, h int) {
	t.Helper()
	rows := strings.Split(m.View(), "\n")
	if len(rows) != h {
		t.Fatalf("%dx%d: frame has %d rows", w, h, len(rows))
	}
	for i, r := range rows {
		if got := lipgloss.Width(r); got != w {
			t.Errorf("%dx%d row %d: width %d != %d", w, h, i, got, w)
		}
	}
}
