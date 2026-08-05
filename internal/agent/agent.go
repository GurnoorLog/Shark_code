package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"shark-agent/internal/provider"
)

const SystemPrompt = `You are SHARKCODE, a terminal coding agent. You help the user build, debug, and run software directly in their terminal.

You have tools to run shell commands, read/write files, list directories, glob, and grep. Use them liberally — do NOT guess. When the user asks you to install dependencies, run the package manager yourself. When they ask to fix a bug, read the relevant files first.

Rules:
- Run commands and inspect outputs before answering.
- Be concise. State what you did and the result.
- Never ask permission to run a read-only command.
- For multi-step work, work through it step by step, using tools.
- If a command fails, read the error and fix it.
`

type Agent struct {
	reg     *provider.Registry
	models  []provider.Provider
	tools   []Tool
	messages []provider.Message
	maxSteps int
}

func New(reg *provider.Registry) *Agent {
	tools := AllTools()
	return &Agent{
		reg:      reg,
		tools:    tools,
		maxSteps: 25,
	}
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

func (a *Agent) Turn(ctx context.Context, userMsg string, onEvent func(TurnEvent)) (string, error) {
	if len(a.messages) == 0 {
		a.messages = append(a.messages, provider.Message{Role: provider.RoleSystem, Content: SystemPrompt})
	}
	a.messages = append(a.messages, provider.Message{Role: provider.RoleUser, Content: userMsg})

	prov := a.reg.Get("")
	if prov == nil {
		return "", fmt.Errorf("no active provider set")
	}

	if onEvent != nil {
		onEvent(TurnEvent{Type: "think", Text: "sharking... " + prov.Name()})
	}

	for step := 0; step < a.maxSteps; step++ {
		resp, err := prov.Chat(ctx, a.messages, ToolDefs(a.tools))
		if err != nil {
			return "", err
		}

		// Record assistant message (text + tool calls).
		assistant := provider.Message{Role: provider.RoleAssistant, Content: resp.Content, ToolCalls: resp.ToolCalls}
		a.messages = append(a.messages, assistant)

		if len(resp.ToolCalls) == 0 {
			return resp.Content, nil
		}

		// Execute each tool call.
		for _, tc := range resp.ToolCalls {
			if onEvent != nil {
				onEvent(TurnEvent{Type: "tool", Tool: tc.Name, Args: tc.Arguments})
			}
			fn, ok := Lookup(a.tools, tc.Name)
			if !ok {
				a.messages = append(a.messages, provider.Message{
					Role: provider.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name,
					Content: fmt.Sprintf("unknown tool %q", tc.Name),
				})
				continue
			}
			var args map[string]any
			json.Unmarshal([]byte(tc.Arguments), &args)
			result, err := fn(ctx, args)
			if err != nil {
				result = "tool error: " + err.Error()
			}
			if onEvent != nil {
				onEvent(TurnEvent{Type: "tool_result", Tool: tc.Name, Result: result})
			}
			a.messages = append(a.messages, provider.Message{
				Role: provider.RoleTool, ToolCallID: tc.ID, ToolName: tc.Name, Content: result,
			})
		}
	}

	return "", fmt.Errorf("max steps (%d) reached", a.maxSteps)
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
