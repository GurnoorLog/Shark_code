package agent

import (
	"context"
	"testing"
)

func TestToolWriteRead(t *testing.T) {
	ctx := context.Background()
	res, err := toolWrite(ctx, map[string]any{"path": "tmp_test_file.txt", "content": "hello shark"})
	if err != nil {
		t.Fatal(err)
	}
	if res == "" {
		t.Fatal("expected write confirmation")
	}
	content, err := toolRead(ctx, map[string]any{"path": "tmp_test_file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	if content != "hello shark" {
		t.Fatalf("expected 'hello shark', got %q", content)
	}
}

func TestToolBash(t *testing.T) {
	ctx := context.Background()
	res, err := toolBash(ctx, map[string]any{"command": "echo sharkcode rocks"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected command output")
	}
}

func TestToolListGlob(t *testing.T) {
	ctx := context.Background()
	_, err := toolListDir(ctx, map[string]any{"path": "."})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := toolGlob(ctx, map[string]any{"pattern": "*.go"})
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) == 0 {
		t.Fatal("expected go files")
	}
}
