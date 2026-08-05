package agent

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"shark-agent/internal/provider"
)

// Tool is a function the model can invoke.
type Tool struct {
	Def   provider.ToolDef
	Run   func(ctx context.Context, args map[string]any) (string, error)
}

func JSONSchema(required []string, props map[string]any) map[string]any {
	return map[string]any{
		"type":       "object",
		"required":   required,
		"properties": props,
	}
}

func StrProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func AllTools() []Tool {
	return []Tool{
		{
			Def: provider.ToolDef{
				Name:        "bash",
				Description: "Run a shell command in the working directory. Use for installing dependencies, running tests, listing files, git, etc.",
				Parameters: JSONSchema([]string{"command"}, map[string]any{
					"command": StrProp("The shell command to run"),
				}),
			},
			Run: toolBash,
		},
		{
			Def: provider.ToolDef{
				Name:        "read_file",
				Description: "Read a file from disk. Returns the contents.",
				Parameters: JSONSchema([]string{"path"}, map[string]any{
					"path": StrProp("Absolute or relative path to the file"),
				}),
			},
			Run: toolRead,
		},
		{
			Def: provider.ToolDef{
				Name:        "write_file",
				Description: "Create or overwrite a file with the given content.",
				Parameters: JSONSchema([]string{"path", "content"}, map[string]any{
					"path":    StrProp("Absolute or relative path to the file"),
					"content": StrProp("Full file content to write"),
				}),
			},
			Run: toolWrite,
		},
		{
			Def: provider.ToolDef{
				Name:        "list_dir",
				Description: "List entries in a directory.",
				Parameters: JSONSchema([]string{"path"}, map[string]any{
					"path": StrProp("Directory to list"),
				}),
			},
			Run: toolListDir,
		},
		{
			Def: provider.ToolDef{
				Name:        "glob",
				Description: "Find files matching a glob pattern.",
				Parameters: JSONSchema([]string{"pattern"}, map[string]any{
					"pattern": StrProp("Glob pattern e.g. **/*.go"),
				}),
			},
			Run: toolGlob,
		},
		{
			Def: provider.ToolDef{
				Name:        "grep",
				Description: "Search file contents for a regex pattern.",
				Parameters: JSONSchema([]string{"pattern"}, map[string]any{
					"pattern": StrProp("Regex to search"),
					"path":    StrProp("Directory or file to search (default: current dir)"),
				}),
			},
			Run: toolGrep,
		},
	}
}

func ToolDefs(tools []Tool) []provider.ToolDef {
	out := make([]provider.ToolDef, len(tools))
	for i, t := range tools {
		out[i] = t.Def
	}
	return out
}

func Lookup(tools []Tool, name string) (func(context.Context, map[string]any) (string, error), bool) {
	for _, t := range tools {
		if t.Def.Name == name {
			return t.Run, true
		}
	}
	return nil, false
}

func toolBash(_ context.Context, args map[string]any) (string, error) {
	cmd := asStr(args["command"])
	if cmd == "" {
		return "", fmt.Errorf("empty command")
	}
	shell, shellArg := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, shellArg = "cmd", "/c"
	}
	c := exec.Command(shell, shellArg, cmd)
	c.Env = os.Environ()
	out, err := c.CombinedOutput()
	s := string(out)
	if len(s) > 8000 {
		s = s[:8000] + "\n...[truncated]"
	}
	if err != nil {
		return s + "\n[exit error] " + err.Error(), nil
	}
	return s, nil
}

func toolRead(_ context.Context, args map[string]any) (string, error) {
	p := asStr(args["path"])
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func toolWrite(_ context.Context, args map[string]any) (string, error) {
	p := asStr(args["path"])
	content := asStr(args["content"])
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("wrote %d bytes to %s", len(content), p), nil
}

func toolListDir(_ context.Context, args map[string]any) (string, error) {
	p := asStr(args["path"])
	if p == "" {
		p = "."
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		b.WriteString(name + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

func toolGlob(_ context.Context, args map[string]any) (string, error) {
	pattern := asStr(args["pattern"])
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "no matches", nil
	}
	return strings.Join(matches, "\n"), nil
}

func toolGrep(_ context.Context, args map[string]any) (string, error) {
	pattern := asStr(args["pattern"])
	root := asStr(args["path"])
	if root == "" {
		root = "."
	}
	var b strings.Builder
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(path, ".git") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.Contains(line, pattern) {
				fmt.Fprintf(&b, "%s:%d: %s\n", path, i+1, strings.TrimSpace(line))
				if b.Len() > 8000 {
					b.WriteString("[truncated]")
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if b.Len() == 0 {
		return "no matches", nil
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

func asStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
