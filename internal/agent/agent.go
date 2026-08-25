package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"log"

	"shark-agent/internal/provider"
)

const SystemPrompt = `You are SHARKCODE, a terminal coding agent that runs in the user's terminal. You act like a senior engineer sitting next to them: you plan, run commands, inspect output, and fix problems yourself.
`

const understandMachineSection = `## First understand the machine
- Greetings, chit-chat, and pure explanation questions get DIRECT answers: no tool calls, no directory inspection, no permission prompts.
- For real work that touches files or runs commands, ground yourself first with ONE quick look (list_dir or read_file on the relevant paths), then act.
- Need system detail (distro, shell version, installed tools)? Run one read-only command to find out instead of assuming.
- If a tool or command goes wrong, TELL the user what failed in one line, then try a different approach. Never stop the whole task because one step failed, and never silently retry the identical failing call.

## How you work
- Use your tools to ground every claim. Never guess about the filesystem, environment, or commands.
- Prefer small, direct shell commands. When the user asks for a task, break it into steps and execute them.
- After every command, read the output and act on it — if it failed, diagnose the error and fix it.
- Only use write_file for actual file contents. To create folders, use the bash tool with mkdir (Windows mkdir works with nested paths).
- Keep answers concise: state what you did and the result. No filler, no preambles, no apologies.

## Ponytail mode: you are a lazy senior dev
Lazy means efficient, never careless. The best code is the code never written. Before writing code, stop at the first rung that holds:
1. Does this need to exist at all? (YAGNI) → skip it, say so in one line.
2. Already in this codebase? → reuse it, don't rewrite.
3. Standard library does it? → use it.
4. Native platform feature covers it? → use it (e.g. <input type="date"> over a picker lib).
5. Already-installed dependency solves it? → use it; never add one for what a few lines can do.
6. Can it be one line? → one line.
7. Only then: the minimum code that works.

The ladder runs AFTER you understand the problem, never instead of it: read the files the change touches and trace the real flow first, then climb.
- Bug fix = root cause, not symptom. Grep every caller of the function you touch; fix the shared function once.
- No abstractions, boilerplate, or dependencies that weren't requested. Deletion over addition. Boring over clever. Fewest files possible.
- Never simplify away: input validation at trust boundaries, error handling that prevents data loss, security, accessibility, or anything explicitly requested.
- Lazy code without its check is unfinished: non-trivial logic leaves ONE runnable check behind (smallest thing that fails if the logic breaks). Trivial one-liners need no test.
- After code: at most three short lines (what was skipped, when to add it). Pattern: [code] → skipped: [X], add when [Y].

## Tool rules
- Read files before modifying them.
- Verify your work when possible (run tests, list the directory, show the file you wrote).
- Prefer read_file/write_file/list_dir/glob/grep over shell for file operations; they behave the same on every OS.
- You have internet access via web_search (find results/URLs), web_fetch (read a page's text), and download_file (save a binary like an image/font/zip to disk). Use web_search ONLY for concrete needs: real image URLs, documentation, JS/CDN links, or exact code you cannot recall. Do NOT web_search "for inspiration" or "for the best design" — inspiration is not output, and searching for it wastes turns. Pick a strong design yourself and build it; if external images are needed, web_search for working source URLs and cite them.
- BINARIES: to place an image (.jpg/.png/.gif/.svg/.webp) or font or zip in your project, call download_file with the real URL and a target path. NEVER pass "[Binary Data]" or "[image]" text to write_file — write_file is TEXT ONLY and will refuse binary markers. edit_file is for text edits in existing files; it takes path/old_string/new_string — it cannot fetch URLs.

## Creating files (IMPORTANT — read carefully)
- When the user asks you to BUILD something (a website, page, app, UI), that is a DIRECT ORDER: do it now, in this session, in one continuous sequence of tool calls. Do NOT reply with a plan, a feature list, inspiration links, or questions like "where do you want to begin" — just build the thing and verify it exists.
- To create any file with content (HTML, CSS, JS, code, config), you MUST call write_file with the FULL file content in its "content" argument and the path in "path". There is no other way to create a file.
- NEVER try to write a file via the bash tool. The bash tool is for running commands, NOT for creating file contents (no "echo ... > file" to build a page, no start-process/notepad/writing HTML through bash). Doing so wastes turns and fails.
- NEVER ask the user a question in the middle of a build. Make reasonable choices yourself and proceed. Only ask when you literally cannot act without their input.
- If write_file reports "refusing to write an empty file", pass the real, complete content and call it again. Do not give up.
- Keep building until the file is actually written and verified: after any write_file, call read_file (or list_dir) to confirm it exists with content. A turn is only "done" when the requested file exists on disk and is non-empty.

## Answer discipline (think token cost)
- Your final answer must be SHORT: 1-3 sentences stating what was built and the exact path(s). Do NOT paste the full file contents, the whole HTML/CSS/JS, or your thinking into the answer — the work lives in the files, not in the reply.
- If you show code for explanation, show a tiny relevant slice (a few lines), never a whole file you just wrote.
- Once the requested work is done and verified, stop. Do not keep re-issuing tools or re-reviewing the same file.

## Debugging (systematic, never guess)
When something fails, STOP adding features and preserve the evidence (error output, repro steps), then:
1. REPRODUCE: make the failure happen reliably. If you can't reproduce it, you can't fix it. Gather logs / environment details first.
2. LOCALIZE: narrow down where it fails (UI / API / DB / build / the test itself). Check the actual traceback, don't guess a layer.
3. Diagnose root cause, not symptom: grep every caller of the function you're about to touch and fix it once in the shared place. A guard in the shared function is a smaller diff than one per caller.
4. FIX, then GUARD against recurrence, then RESUME.
Don't push past a failing build/test to work on the next feature — errors compound.

## Code review before finishing
Review your own change across the axes before declaring done:
- CORRECTNESS: does it do what was asked? Edge cases (empty, null, boundary)? Error paths, not just happy path? Does it pass the tests, and are the tests testing the right thing?
- READABILITY: descriptive names, follows existing conventions. No opaque "temp"/"data".
- ARCHITECTURE: fits the existing structure; no duplication (is there already a helper doing this?).
- SECURITY: validate inputs at trust boundaries; no secrets committed; no path/traversal or injection.
- PERFORMANCE: no obvious waste (re-reading files, O(n²) over data that will grow).
Approve when it improves overall health even if imperfect; don't leave the codebase worse.

## Precision
- When asked to create something, verify it was created (e.g. list the folder with dir or list_dir).
- Do not invent tool arguments that the tool schemas don't define.
- If you are unsure, run a command to find out.
- If a tool errors, read the message, pick the CORRECT tool, and retry. Never stop a task just because one attempt failed — diagnose and continue until the goal is achieved.

Never ask permission to run a read-only or inspection command.
`

const windowsSection = `## You are on Windows
- The "bash" tool actually invokes cmd.exe (Command Prompt), NOT bash.
- cmd.exe does NOT support: ls, cat, cp, mv, rm -rf, mkdir -p, touch, which, grep (standalone), pwd, apt/brew/yum.
- Use Windows equivalents: dir, type, copy, move, del, rmdir /s /q, mkdir. Common translations are applied automatically, but write native commands when you can.
- NEVER use "~" in a bash command: cmd.exe does not expand it. Use the absolute home path from "This machine" below.
- If mkdir says "already exists", the folder exists; that is fine, move on.`

const darwinSection = `## You are on macOS
- The "bash" tool runs commands through /bin/sh on a Unix system; standard tools work: ls, cat, grep, cp, mv, rm -rf, mkdir -p, touch, which, pwd.
- "~" expands to the real home directory below. Prefer forward-slash paths everywhere.
- Only reach for brew when something genuinely needs installing; prefer what already exists.`

const linuxSection = `## You are on Linux
- The "bash" tool runs commands through /bin/sh on a Unix system; standard tools work: ls, cat, grep, cp, mv, rm -rf, mkdir -p, touch, which, pwd.
- "~" expands to the real home directory below. Prefer forward-slash paths everywhere.
- Only use the distro package manager (apt/dnf/pacman) when something genuinely needs installing; prefer what already exists.`

func platformSection() string {
	switch runtime.GOOS {
	case "windows":
		return windowsSection
	case "darwin":
		return darwinSection
	default:
		return linuxSection
	}
}

func shellName() string {
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	return "sh"
}

func detectOSVersion() string {
	switch runtime.GOOS {
	case "windows":
		return runProbe("cmd", "/c", "ver")
	case "darwin":
		if out := runProbe("sw_vers"); out != "" {
			return out
		}
		return runProbe("uname", "-mr")
	default:
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			for _, ln := range strings.Split(string(data), "\n") {
				if strings.HasPrefix(ln, "PRETTY_NAME=") {
					return strings.Trim(strings.TrimPrefix(ln, "PRETTY_NAME="), `"`)
				}
			}
		}
		return runProbe("uname", "-sr")
	}
}

func runProbe(name string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (a *Agent) buildSystemPrompt() string {
	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	user := os.Getenv("USERNAME")
	if user == "" {
		user = os.Getenv("USER")
	}

	var b strings.Builder
	b.WriteString(SystemPrompt)
	b.WriteString("\n")
	b.WriteString(platformSection())
	b.WriteString("\n\n")
	b.WriteString(understandMachineSection)
	b.WriteString("\n\n## This machine (authoritative, do not guess)\n")
	b.WriteString(fmt.Sprintf("- Operating system: %s (%s)\n", runtime.GOOS, runtime.GOARCH))
	b.WriteString(fmt.Sprintf("- Shell used by the bash tool: %s\n", shellName()))
	b.WriteString(fmt.Sprintf("- Real home directory: %s\n", home))
	b.WriteString(fmt.Sprintf("- Real current working directory: %s\n", cwd))
	b.WriteString(fmt.Sprintf("- Real username: %s\n", user))
	b.WriteString(a.probeFacts())
	b.WriteString("- Use these EXACT paths above. Never invent a username or folder.\n")
	b.WriteString("- Example that will FAIL on Windows: C:\\Users\\<wrongname>\\Desktop\\... The home dir is the real one above.\n")
	b.WriteString("- When a mkdir/write fails with access denied, check you used the real home dir and that the parent folder exists.\n")
	return b.String()
}

func (a *Agent) probeFacts() string {
	if !a.sysProbed {
		a.sysProbe = detectOSVersion()
		a.sysProbed = true
	}
	if a.sysProbe == "" {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "- OS version:\n")
	for _, ln := range strings.Split(a.sysProbe, "\n") {
		fmt.Fprintf(&b, "    %s\n", ln)
	}
	return b.String()
}

type Agent struct {
	reg      *provider.Registry
	models   []provider.Provider
	tools    []Tool
	messages []provider.Message
	maxSteps int

	// usage accumulates token counts across turns for the context panel.
	Usage Usage

	// Verify enables the accuracy gate: after producing an answer, the
	// model is asked to diagnose failures and fix them before finalizing.
	Verify bool

	// Plan, when true, restricts the agent to read-only tools (plan mode).
	// Mutating tools (write_file, edit_file, delete_file, download_file,
	// and bash) are blocked so the model plans without changing anything.
	Plan bool

	usedMutating bool

	failStreak int

	sysProbe   string
	sysProbed  bool

	// Confirm, if set, is called before each tool executes. It may block
	// (e.g. waiting for the user to approve). Returning false skips the
	// tool and tells the model the action was denied.
	Confirm func(req ConfirmRequest) bool
}

// ConfirmRequest describes a tool execution that needs approval.
type ConfirmRequest struct {
	Tool string
	Args string
}

func New(reg *provider.Registry) *Agent {
	tools := AllTools()
	return &Agent{
		reg:      reg,
		tools:    tools,
		maxSteps: 40,
		Verify:   true,
	}
}

func (a *Agent) addUsage(providerName, model string, in, out int) {
	a.Usage.InputTokens += in
	a.Usage.OutputTokens += out
	a.Usage.CostUSD += estimateCostUSD(providerName, model, in, out)
}

// ContextWindow returns the active model's context size for the % bar.
func (a *Agent) ContextWindow() int {
	if a.reg == nil {
		return 128_000
	}
	p := a.reg.Get("")
	if p == nil {
		return 128_000
	}
	return contextWindowFor(p.Model())
}

// Messages returns a copy of the conversation history.
func (a *Agent) Messages() []provider.Message {
	return append([]provider.Message{}, a.messages...)
}

func (a *Agent) SetMessages(msgs []provider.Message) {
	a.messages = append([]provider.Message{}, msgs...)
}

func (a *Agent) Clear() {
	a.messages = nil
	a.Usage = Usage{}
}

// Turn runs one full agent turn: model call, tool execution loop, final answer.
// The callback is invoked with progress events (tool calls, partial text).
type TurnEvent struct {
	Type    string // "tool", "tool_result", "think", "answer"
	Tool    string
	Args    string
	Result  string
	Text    string
}

func ParseArgs(raw string) map[string]any {
	var m map[string]any
	if len(raw) == 0 {
		return m
	}
	if err := json.Unmarshal([]byte(raw), &m); err == nil {
		return m
	}
	var s string
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		json.Unmarshal([]byte(s), &m)
	}
	return m
}

func (a *Agent) Turn(ctx context.Context, userMsg string, onEvent func(TurnEvent)) (string, error) {
	if len(a.messages) == 0 {
		a.messages = append(a.messages, provider.Message{Role: provider.RoleSystem, Content: a.buildSystemPrompt()})
	}
	// In plan mode, prefix the user prompt with plan-only instructions so
	// the guidance lives in this turn's request and doesn't linger in the
	// history after the user switches back to build mode.
	if a.Plan {
		userMsg = "PLAN MODE (mutating tools disabled — do not create/edit/delete/download anything): inspect as needed, then produce a concise step-by-step plan with exact file paths, what you'd change, and why.\n\nUSER REQUEST: " + userMsg
	}
	a.messages = append(a.messages, provider.Message{Role: provider.RoleUser, Content: userMsg})

	prov := a.reg.Get("")
	if prov == nil {
		return "", fmt.Errorf("no active provider set")
	}

	if onEvent != nil {
		onEvent(TurnEvent{Type: "think", Text: "sharking... " + prov.Name()})
	}

	content, err := a.runLoop(ctx, prov, onEvent, true)
	if err != nil {
		return "", err
	}

	// Accuracy gate: only when the turn actually used tools, run a
	// diagnosis/fix pass. Without this guard the gate asks the model to
	// "review the work above" on every turn, which makes small models
	// hallucinate fake tasks (e.g. greeting turns inventing file work).
	if a.Verify && !a.Plan && len(content) > 0 && a.usedMutating {
		if onEvent != nil {
			onEvent(TurnEvent{Type: "think", Text: "gate: verifying & diagnosing..."})
		}
		a.messages = append(a.messages, provider.Message{
			Role:    provider.RoleUser,
			Content: "Accuracy gate: critically review the task and your work above. Diagnose any failures, mistakes, or unfinished parts, and FIX them using your tools. When everything is correct and complete, give the final answer.",
		})
		a.usedMutating = false
		fixed, ferr := a.runLoop(ctx, prov, onEvent, false)
		if ferr == nil && strings.TrimSpace(fixed) != "" {
			content = fixed
		}
	}

	return content, nil
}

func safeCall(fn func(context.Context, map[string]any) (string, error), ctx context.Context, args map[string]any) (res string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool crashed: %v", r)
		}
	}()
	return fn(ctx, args)
}

// runLoop drives the model call -> tool execution loop until a plain
// text answer is produced or the step budget is exhausted.
func (a *Agent) runLoop(ctx context.Context, prov provider.Provider, onEvent func(TurnEvent), emitToolEvents bool) (string, error) {
	for step := 0; step < a.maxSteps; step++ {
		// Use token streaming when the provider supports it so the UI
		// shows the reply live; otherwise fall back to a single shot.
		var resp *provider.Response
		var err error
		// Some small models occasionally return a completely empty
		// response (no text, no tool calls). Retry a couple of times so a
		// blank turn doesn't silently end the task.
		for attempt := 0; ; attempt++ {
			call := func() (*provider.Response, error) {
				defs := ToolDefs(a.tools)
				if a.Plan {
					defs = planDefs(a.tools)
				}
				if s, ok := prov.(provider.Streamer); ok && emitToolEvents {
					return s.ChatStream(ctx, a.messages, defs, func(delta string) {
						if onEvent != nil {
							onEvent(TurnEvent{Type: "token", Text: delta})
						}
					})
				}
				return prov.Chat(ctx, a.messages, defs)
			}
			resp, err = call()
			if err != nil {
				if attempt < 2 && ctx.Err() == nil {
					if onEvent != nil {
						onEvent(TurnEvent{Type: "think", Text: fmt.Sprintf("provider hiccup (%s) — retrying...", short(err.Error(), 60))})
					}
					time.Sleep(time.Duration(attempt+1) * 800 * time.Millisecond)
					continue
				}
				return "", fmt.Errorf("provider %s unreachable after retries: %w", prov.Name(), err)
			}
			if strings.TrimSpace(resp.Content) != "" || len(resp.ToolCalls) > 0 || attempt >= 2 {
				break
			}
			if onEvent != nil {
				onEvent(TurnEvent{Type: "think", Text: "blank reply — nudging the model..."})
			}
		}
		a.addUsage(prov.Name(), prov.Model(), resp.InputTokens, resp.OutputTokens)

		// Record assistant message (text + tool calls).
		assistant := provider.Message{Role: provider.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls}
		a.messages = append(a.messages, assistant)

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Execute each tool call.
	for _, tc := range resp.ToolCalls {
		if emitToolEvents && onEvent != nil {
			onEvent(TurnEvent{Type: "tool", Tool: tc.Name, Args: tc.Arguments})
		}
		// In plan mode, mutating tools are blocked outright — the model may
		// read and analyze but cannot modify anything.
		if a.Plan && isMutating(tc.Name) {
			a.messages = append(a.messages, provider.Message{
				Role: provider.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name,
				Content: fmt.Sprintf("PLAN MODE: %s is a mutating tool and was blocked. Explain in your plan what %s would do and where, without executing it.", tc.Name, tc.Name),
			})
			continue
		}
		fn, ok := Lookup(a.tools, tc.Name)
			if !ok {
				a.messages = append(a.messages, provider.Message{
					Role: provider.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name,
					Content: fmt.Sprintf("unknown tool %q", tc.Name),
				})
				continue
			}
			if !isReadOnly(tc.Name) {
				a.usedMutating = true
				if a.Confirm != nil && !a.Confirm(ConfirmRequest{Tool: tc.Name, Args: tc.Arguments}) {
					a.messages = append(a.messages, provider.Message{
						Role: provider.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name,
						Content: "The user denied this action. Do not attempt it again; explain what you need permission for instead.",
					})
					continue
				}
			}
			args := ParseArgs(tc.Arguments)
			result, err := safeCall(fn, ctx, args)
			if err != nil {
				a.failStreak++
				result = "tool error: " + err.Error()
				if a.failStreak >= 3 {
					if onEvent != nil && emitToolEvents {
						onEvent(TurnEvent{Type: "think", Text: fmt.Sprintf("%s failed %d times — switching approach", tc.Name, a.failStreak)})
					}
					result += fmt.Sprintf("\n(note: %d tool calls failed in a row. Stop repeating the same call. Diagnose first: read the error, verify the path/input exists with list_dir or read_file, or use a different command/tool.)", a.failStreak)
				}
			} else {
				a.failStreak = 0
			}
			if emitToolEvents && onEvent != nil {
				onEvent(TurnEvent{Type: "tool_result", Tool: tc.Name, Result: result})
			}
			a.messages = append(a.messages, provider.Message{
				Role: provider.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name, Content: result,
			})
		}
	}

	if onEvent != nil {
		onEvent(TurnEvent{Type: "error", Text: fmt.Sprintf("hit the step budget (%d steps). Task paused, not failed — say \"continue\" to keep going.", a.maxSteps)})
	}
	return "", nil
}

// GetModel returns the active provider/model name for display.
func (a *Agent) GetModel() string {
	p := a.reg.Get("")
	if p == nil {
		return "no provider"
	}
	return p.Name()
}

var _ = log.Println

