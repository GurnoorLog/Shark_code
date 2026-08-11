package agent

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
				Description: "Run a shell command in the working directory. Use for installing dependencies, running tests, git, creating directories (mkdir), copying/moving/deleting files, etc.",
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
					"path": StrProp("Absolute or relative path to the file (supports ~ for home)"),
				}),
			},
			Run: toolRead,
		},
		{
			Def: provider.ToolDef{
				Name:        "write_file",
				Description: "Create or overwrite a FILE with the given content. Do NOT use this to create a folder/directory — use the bash tool with mkdir for that.",
				Parameters: JSONSchema([]string{"path", "content"}, map[string]any{
					"path":    StrProp("Absolute or relative path to the file (supports ~ for home)"),
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
					"path": StrProp("Directory to list (supports ~ for home)"),
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
		{
			Def: provider.ToolDef{
				Name:        "edit_file",
				Description: "Find an exact old string in a file and replace it with a new string. The old string must be unique in the file. Use this for targeted edits instead of rewriting whole files.",
				Parameters: JSONSchema([]string{"path", "old_string", "new_string"}, map[string]any{
					"path":       StrProp("Path to the file to edit (supports ~ for home)"),
					"old_string": StrProp("Exact text to find (must be unique)"),
					"new_string": StrProp("Replacement text"),
				}),
			},
			Run: toolEdit,
		},
		{
			Def: provider.ToolDef{
				Name:        "delete_file",
				Description: "Delete a file (not a directory).",
				Parameters: JSONSchema([]string{"path"}, map[string]any{
					"path": StrProp("Path to the file to delete (supports ~ for home)"),
				}),
			},
			Run: toolDelete,
		},
		{
			Def: provider.ToolDef{
				Name:        "web_fetch",
				Description: "Fetch the text content of a URL from the internet. Use to read documentation, grab an image URL, check an API, or verify something exists online.",
				Parameters: JSONSchema([]string{"url"}, map[string]any{
					"url": StrProp("The full http(s) URL to fetch"),
				}),
			},
			Run: toolWebFetch,
		},
		{
			Def: provider.ToolDef{
				Name:        "download_file",
				Description: "Download a binary or file from a URL and save it to a local path (e.g. images, videos, zip, fonts). Use THIS for binary assets — never write_file with [Binary Data] or placeholder text. For .jpg/.png/.zip etc. use download_file, not web_fetch.",
				Parameters: JSONSchema([]string{"url", "path"}, map[string]any{
					"url":  StrProp("The full http(s) URL to download from"),
					"path": StrProp("Absolute or relative local path to save the file to (supports ~ for home)"),
				}),
			},
			Run: toolDownload,
		},
		{
			Def: provider.ToolDef{
				Name:        "web_search",
				Description: "Search the internet for a query and return the top results with titles, snippets, and URLs. Use to find image URLs, documentation, libraries, or answers.",
				Parameters: JSONSchema([]string{"query"}, map[string]any{
					"query": StrProp("The search query"),
				}),
			},
			Run: toolWebSearch,
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

// mutatingTools is the set of tools that change the filesystem/shell state.
// They are hidden and blocked in plan mode.
var mutatingTools = map[string]bool{
	"bash":          true,
	"write_file":    true,
	"edit_file":     true,
	"delete_file":   true,
	"download_file": true,
}

func isMutating(name string) bool { return mutatingTools[name] }

// planDefs returns only the read-only tool definitions, so a model in plan
// mode sees just the inspection tools and can't drift toward mutating ones.
func planDefs(tools []Tool) []provider.ToolDef {
	var out []provider.ToolDef
	for _, t := range tools {
		if isMutating(t.Def.Name) {
			continue
		}
		out = append(out, t.Def)
	}
	return out
}

func toolBash(_ context.Context, args map[string]any) (string, error) {
	cmd := asStr(args["command"])
	if cmd == "" {
		return "", fmt.Errorf("empty command")
	}
	trimmed := strings.TrimSpace(cmd)
	if strings.HasPrefix(trimmed, "-") {
		return "", fmt.Errorf("invalid command %q: it starts with a flag and has no executable. Did you drop the program name? Example: mkdir C:\\Users\\<name>\\Desktop\\myfolder  (not \"-p C:\\...\")", cmd)
	}
	shell, shellArg := "sh", "-c"
	if runtime.GOOS == "windows" {
		shell, shellArg = "cmd", "/c"
		cmd = windowsCmd(cmd)
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

// windowsCmd translates common Unix shell idioms so the agent's commands
// actually work when bash runs under cmd.exe on Windows.
func windowsCmd(cmd string) string {
	home, herr := os.UserHomeDir()
	hadTilde := false
	if herr == nil {
		cmd = strings.ReplaceAll(cmd, "~/", home+`\`)
		cmd = strings.ReplaceAll(cmd, `~\`, home+`\`)
		hadTilde = strings.Contains(cmd, home)
	}
	_ = hadTilde
	cmd = strings.ReplaceAll(cmd, "/", `\`)
	cmd = strings.ReplaceAll(cmd, "mkdir -p ", "mkdir ")
	cmd = strings.ReplaceAll(cmd, "rm -rf ", "rmdir /s /q ")
	cmd = strings.ReplaceAll(cmd, "rm -r ", "rmdir /s /q ")
	cmd = strings.ReplaceAll(cmd, "touch ", "type nul > ")
	cmd = strings.ReplaceAll(cmd, "&& ", "&& ")
	return cmd
}

func toolRead(_ context.Context, args map[string]any) (string, error) {
	p := expandPath(asStr(args["path"]))
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func toolWrite(_ context.Context, args map[string]any) (string, error) {
	p := expandPath(asStr(args["path"]))
	content := asStr(args["content"])
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("refusing to write an empty file to %s: content is empty. Provide the real file content in the content argument, then call write_file again.", p)
	}
	// Refuse obvious binary/placeholder stubs — you cannot hand an image to
	// write_file as [Binary Data]. Use web_fetch to download binaries.
	low := strings.ToLower(content)
	if strings.Contains(low, "[binary") || strings.Contains(low, "[image") || strings.Contains(low, "\x00") {
		return "", fmt.Errorf("refusing to write %s: you passed a binary/placeholder marker (%q) as text content. write_file writes TEXT only. To download a binary file (jpg/png/etc.) use download_file, which saves bytes to disk directly.", p, short(content, 30))
	}
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
	p := expandPath(asStr(args["path"]))
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
	pattern := expandPath(asStr(args["pattern"]))
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
	root := expandPath(asStr(args["path"]))
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

// short truncates a string for error messages.
func short(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func toolEdit(_ context.Context, args map[string]any) (string, error) {
	p := expandPath(asStr(args["path"]))
	oldS := asStr(args["old_string"])
	newS := asStr(args["new_string"])
	if oldS == "" {
		return "", fmt.Errorf("old_string must not be empty")
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	content := string(data)
	n := strings.Count(content, oldS)
	if n == 0 {
		return "", fmt.Errorf("old_string not found in %s", p)
	}
	if n > 1 {
		return "", fmt.Errorf("old_string appears %d times in %s; include more context to make it unique", n, p)
	}
	content = strings.Replace(content, oldS, newS, 1)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("edited %s (%d replacement)", p, n), nil
}

func toolDelete(_ context.Context, args map[string]any) (string, error) {
	p := expandPath(asStr(args["path"]))
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("delete_file only removes files; use bash rmdir for directories")
	}
	if err := os.Remove(p); err != nil {
		return "", err
	}
	return "deleted " + p, nil
}

var webClient = &http.Client{Timeout: 30 * time.Second}

// toolWebFetch downloads the body of a URL as plain text (HTML stripped of
// tags for readability) so the model can read docs / verify content online.
func toolWebFetch(ctx context.Context, args map[string]any) (string, error) {
	u := asStr(args["url"])
	if u == "" {
		return "", fmt.Errorf("empty url")
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "https://" + u
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) SHARKCODE/1.0")
	resp, err := webClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("web_fetch %s: status %d", u, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if err != nil {
		return "", err
	}
	ct := resp.Header.Get("Content-Type")
	// Images and binaries aren't readable as text.
	if !strings.HasPrefix(ct, "text/") && !strings.Contains(ct, "json") && !strings.Contains(ct, "xml") &&
		!strings.Contains(ct, "javascript") && ct != "" && !strings.Contains(ct, "html") {
		return fmt.Sprintf("fetched %s (%s, %d bytes) — binary content, not rendered as text", u, ct, len(body)), nil
	}
	text := stripHTML(string(body))
	if len(text) > 6000 {
		text = text[:6000] + "\n...[truncated]"
	}
	return text, nil
}

// toolDownload fetches a URL and saves its raw bytes to a local path.
// Use for binary assets (images, zips, fonts) that web_fetch can't render.
func toolDownload(ctx context.Context, args map[string]any) (string, error) {
	u := asStr(args["url"])
	if u == "" {
		return "", fmt.Errorf("empty url")
	}
	p := expandPath(asStr(args["path"]))
	if p == "" {
		return "", fmt.Errorf("empty path")
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = "https://" + u
	}
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) SHARKCODE/1.0")
	resp, err := webClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("download_file %s: status %d", u, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 50_000_000))
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return "", err
	}
	return fmt.Sprintf("downloaded %d bytes from %s → %s", len(body), u, p), nil
}

// stripHTML removes tags and collapses whitespace for a readable text view.
func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	i := 0
	n := len(s)
	skipContent := false
	for i < n {
		if inTag {
			if s[i] == '>' {
				inTag = false
			}
			i++
			continue
		}
		if s[i] == '<' {
			if name, ok := tagNameAt(s, i); ok {
				switch name {
				case "script", "style", "title", "head", "noscript":
					skipContent = true
				case "/script", "/style", "/title", "/head", "/noscript":
					skipContent = false
				}
			}
			inTag = true
			i++
			continue
		}
		if !skipContent {
			b.WriteByte(s[i])
		}
		i++
	}
	return collapseWS(b.String())
}

// tagNameAt reads a tag name starting at '<' at index i in s. Returns the
// name (possibly with a leading '/') and whether a valid name was found.
func tagNameAt(s string, i int) (string, bool) {
	j := i + 1
	if j >= len(s) {
		return "", false
	}
	if s[j] == '/' {
		j++
	}
	start := j
	for j < len(s) {
		c := s[j]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			j++
			continue
		}
		break
	}
	name := strings.ToLower(s[start:j])
	if name == "" {
		return "", false
	}
	if start > i+1 { // had a '/'
		return "/" + name, true
	}
	return name, true
}

// collapseWS condenses runs of whitespace into a single space.
func collapseWS(s string) string {
	var sb strings.Builder
	space := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !space {
				sb.WriteByte(' ')
				space = true
			}
		} else {
			sb.WriteByte(c)
			space = false
		}
	}
	return strings.TrimSpace(sb.String())
}

// toolWebSearch searches the web via a lightweight query URL and returns
// the first few HTML results as readable lines.
func toolWebSearch(ctx context.Context, args map[string]any) (string, error) {
	q := asStr(args["query"])
	if q == "" {
		return "", fmt.Errorf("empty query")
	}
	// DuckDuckGo HTML (no JS required) is the most scraping-friendly.
	u := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) SHARKCODE/1.0")
	resp, err := webClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("web_search: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2_000_000))
	if err != nil {
		return "", err
	}
	return parseDDGResults(string(body)), nil
}

// parseDDGResults extracts result titles and links from DuckDuckGo HTML.
func parseDDGResults(html string) string {
	var out []string
	idx := 0
	for {
		// result links look like <a rel="nofollow" class="result__a" href="...">Title</a>
		s := strings.Index(html[idx:], "result__a")
		if s < 0 {
			break
		}
		s += idx
		// find href
		h := strings.Index(html[s:], "href=")
		if h < 0 {
			break
		}
		h += s
		h = strings.Index(html[h:], `"`) + h + 1
		he := strings.Index(html[h:], `"`) + h
		link := html[h:he]
		// title text follows after '>'
		t := strings.Index(html[he:], ">") + he + 1
		te := strings.Index(html[t:], "<") + t
		title := strings.TrimSpace(stripHTML(html[t:te]))
		link = decodeDDGLink(link)
		if link != "" && title != "" {
			out = append(out, title+" - "+link)
		}
		if len(out) >= 8 {
			break
		}
		idx = te
	}
	if len(out) == 0 {
		return "(web_search: no results parsed)"
	}
	return strings.Join(out, "\n")
}

// decodeDDGLink resolves DuckDuckGo's redirect URL back to the real link.
func decodeDDGLink(link string) string {
	if !strings.Contains(link, "/l/?uddg=") {
		return link
	}
	i := strings.Index(link, "uddg=")
	enc := link[i+len("uddg="):]
	if amp := strings.Index(enc, "&"); amp >= 0 {
		enc = enc[:amp]
	}
	dec, err := url.QueryUnescape(enc)
	if err != nil {
		return link
	}
	return dec
}

// expandPath resolves a leading "~" or "$HOME" to the user's home directory.
func expandPath(p string) string {
	if p == "" {
		return p
	}
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return home
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return os.ExpandEnv(p)
}
