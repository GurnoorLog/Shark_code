package agent

import (
	"strings"
	"testing"
)

func TestSystemPromptOrder(t *testing.T) {
	a := New(nil)
	p := a.buildSystemPrompt()

	iPlatform := strings.Index(p, "## You are on")
	if iPlatform < 0 {
		t.Fatal("platform section missing from system prompt")
	}
	iGround := strings.Index(p, "## First understand the machine")
	iRules := strings.Index(p, "## How you work")
	iMachine := strings.Index(p, "## This machine")
	for _, tc := range []struct {
		name string
		idx  int
	}{{"grounding", iGround}, {"rules", iRules}, {"machine facts", iMachine}} {
		if tc.idx < 0 {
			t.Fatalf("%s section missing", tc.name)
		}
		if tc.idx < iPlatform {
			t.Errorf("%s section should come after platform rules", tc.name)
		}
	}

	if !strings.Contains(p, "no tool calls") {
		t.Error("grounding rules no longer scope inspections away from greetings")
	}
}
