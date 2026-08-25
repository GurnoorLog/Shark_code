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

	for _, sec := range []string{
		"## Step 1 - classify every request",
		"## Step 2 - work the task one tool call at a time",
		"## Step 3 - finish properly",
	} {
		i := strings.Index(p, sec)
		if i < 0 {
			t.Fatalf("section %q missing", sec)
		}
		if i > iPlatform && strings.HasPrefix(sec, "## Step") {
			t.Errorf("section %q must come before platform rules so it is read first", sec)
		}
	}

	iMachine := strings.Index(p, "## This machine")
	if iMachine < 0 || iMachine < iPlatform {
		t.Fatal("machine facts must follow the platform rules")
	}

	for _, want := range []string{"answer directly. No tools", "NEVER end your turn mid-task", "mkdir"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing rule containing %q", want)
		}
	}
}
