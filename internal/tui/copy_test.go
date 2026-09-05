package tui

import "testing"

func TestNthAssistant(t *testing.T) {
	entries := []entry{
		{kind: "user", text: "hi"},
		{kind: "assistant", text: "first answer"},
		{kind: "tool", text: "bash echo"},
		{kind: "assistant", text: "second answer"},
	}

	for tidx, tc := range []struct {
		n    int
		want string
		ok   bool
	}{
		{1, "second answer", true},
		{2, "first answer", true},
		{3, "", false},
		{0, "", false},
	} {
		got, ok := nthAssistant(entries, tc.n)
		if got != tc.want || ok != tc.ok {
			t.Errorf("case %d: nthAssistant(%d) = (%q, %v), want (%q, %v)", tidx, tc.n, got, ok, tc.want, tc.ok)
		}
	}

	if _, ok := nthAssistant(nil, 1); ok {
		t.Error("nil entries should not match")
	}
}