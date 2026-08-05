package provider

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolName   string
	ToolCalls  []ToolCall
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type ToolDef struct {
	Name        string
	Description string
	Parameters  map[string]any
}

type Response struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	Tokens       int
}

type Provider interface {
	Name() string
	Chat(ctx context.Context, msgs []Message, tools []ToolDef) (*Response, error)
}

type Tool struct {
	ID   string
	Name string
	Args string
}
