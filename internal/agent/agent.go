package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"log"

	"shark-agent/internal/provider"
)

const SystemPrompt = `You are SHARKCODE, a senior engineer living in the user's terminal. You plan, run commands, inspect output, and fix problems yourself until the job is done.

## Step 1 - classify every request
- CHAT ("hi", thanks, opinions, small talk): answer directly. No tools.
- QUESTION about the code, system, or docs ("how does X work here", "what version is installed"): do 1-3 quick read-only lookups (list_dir, read_file, grep, bash) and answer from what you saw. Never guess what you can check.
- TASK (build, create, fix, change, run something): go work it in Step 2.

## Step 2 - work the task one tool call at a time
- Break it into steps and execute them in order.
- FOLDERS are created with the bash tool (mkdir). FILES with content are created with write_file. Nothing else.
- Read before editing: read_file opens the file before you edit_file or overwrite it.
- After every tool call, READ THE RESULT. If it failed: state what failed in one line, diagnose, then act DIFFERENTLY. Never repeat an identical failing call.
- NEVER end your turn mid-task by announcing next steps ("now let's...", "next I will..."). If there is a next step, call the tool in this same response. Plain text is ONLY the final answer when everything is done and verified.
- Do not ask the user questions mid-task; make sensible choices and proceed.
- Internet: web_search finds URLs/docs, web_fetch reads pages, download_file saves binaries (images, fonts, zips). Search only for concrete needs, never "for inspiration". Binary assets NEVER go through write_file.

## Step 3 - finish properly
- VERIFY: run what you built or list/read what you wrote. Done means it exists on disk and works.
- Final answer: 1-3 sentences - what was done and the exact path(s). No file dumps, no thinking out loud, then stop.

## Style
Lazy senior dev, not careless junior: the minimum code that fully works. Reuse over rewrite over new. Standard library over dependencies. Fix root causes, not symptoms (grep the callers, fix the shared place once). No unrequested abstractions. Non-trivial logic leaves one small runnable check behind. When debugging: reproduce, localize, fix the root cause, resume - never push past a failing build to build more on top.
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

	toolRan bool

	failStreak int

	sysProbe  string
	sysProbed bool

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
	Type   string // "tool", "tool_result", "think", "answer"
	Tool   string
	Args   string
	Result string
	Text   string
}

var announceRe = regexp.MustCompile(`(?i)\b(let'?s|let us|now (i|we)|i will|i'll|going to|first,? (i|we|let)|create it first|next,? (i|we|let)|about to)\b`)

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

	a.usedMutating = false
	a.toolRan = false
	a.failStreak = 0

	if onEvent != nil {
		onEvent(TurnEvent{Type: "think", Text: "sharking... " + prov.Name()})
	}

	content, err := a.runLoop(ctx, prov, onEvent, true)
	if err != nil {
		return "", err
	}

	if !a.Plan && a.toolRan && len(content) > 0 {
		tail := content
		if len(tail) > 160 {
			tail = tail[len(tail)-160:]
		}
		if announceRe.MatchString(tail) {
			if onEvent != nil {
				onEvent(TurnEvent{Type: "think", Text: "model announced next steps instead of acting — continuing..."})
			}
			a.messages = append(a.messages, provider.Message{
				Role:    provider.RoleUser,
				Content: "Your last message announced what you were about to do, but you stopped without doing it. Continue the task NOW by calling the tools. Only send plain text when the entire task is finished and verified.",
			})
			cont, cerr := a.runLoop(ctx, prov, onEvent, true)
			if cerr == nil && strings.TrimSpace(cont) != "" {
				content = cont
			}
		}
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
			a.toolRan = true
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
