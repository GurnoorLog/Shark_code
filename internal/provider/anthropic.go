package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"shark-agent/internal/config"
)

// Anthropic implements the Anthropic Messages API for Claude models.
type Anthropic struct {
	cfg    *config.ProviderConfig
	client *http.Client
}

func NewAnthropic(cfg *config.ProviderConfig) *Anthropic {
	return &Anthropic{
		cfg:    cfg,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *Anthropic) Name() string { return "anthropic" }

func (p *Anthropic) Model() string { return p.cfg.Model }

type anMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anToolUseBlock struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

type anToolResultBlock struct {
	Type        string `json:"type"`
	ToolUseID   string `json:"tool_use_id"`
	Content     string `json:"content"`
	IsError     bool   `json:"is_error"`
}

type anTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
}

type anReq struct {
	Model     string    `json:"model"`
	MaxTokens int       `json:"max_tokens"`
	System    string    `json:"system,omitempty"`
	Messages  []anMsg   `json:"messages"`
	Tools     []anTool  `json:"tools,omitempty"`
}

type anResp struct {
	Content []struct {
		Type  string `json:"type"`
		Text  string `json:"text,omitempty"`
		ID    string `json:"id,omitempty"`
		Name  string `json:"name,omitempty"`
		Input map[string]any `json:"input,omitempty"`
	} `json:"content"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

func (p *Anthropic) Chat(ctx context.Context, msgs []Message, tools []ToolDef) (*Response, error) {
	var system string
	var anMsgs []anMsg

	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			system += m.Content + "\n"
		case RoleUser:
			if m.ToolCallID == "" {
				b, _ := json.Marshal([]anTextBlock{{Type: "text", Text: m.Content}})
				anMsgs = append(anMsgs, anMsg{Role: "user", Content: b})
			} else {
				b, _ := json.Marshal([]anToolResultBlock{{
					Type: "tool_result", ToolUseID: m.ToolCallID, Content: m.Content,
				}})
				anMsgs = append(anMsgs, anMsg{Role: "user", Content: b})
			}
		case RoleAssistant:
			var blocks []json.RawMessage
			if m.Content != "" {
				blocks = append(blocks, mustJSON(anTextBlock{Type: "text", Text: m.Content}))
			}
			for _, tc := range m.ToolCalls {
				var input map[string]any
				json.Unmarshal([]byte(tc.Arguments), &input)
				blocks = append(blocks, mustJSON(anToolUseBlock{
					Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: input,
				}))
			}
			joined, _ := json.Marshal(blocks)
			anMsgs = append(anMsgs, anMsg{Role: "assistant", Content: joined})
		}
	}

	var anTools []anTool
	for _, t := range tools {
		anTools = append(anTools, anTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.Parameters,
		})
	}

	body, _ := json.Marshal(anReq{
		Model:     p.cfg.Model,
		MaxTokens: 8192,
		System:    system,
		Messages:  anMsgs,
		Tools:     anTools,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.cfg.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.cfg.APIKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("anthropic: %d %s", resp.StatusCode, string(raw))
	}

	var ar anResp
	if err := json.Unmarshal(raw, &ar); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	out := &Response{FinishReason: ar.StopReason, Tokens: ar.Usage.InputTokens + ar.Usage.OutputTokens, InputTokens: ar.Usage.InputTokens, OutputTokens: ar.Usage.OutputTokens}
	for _, c := range ar.Content {
		switch c.Type {
		case "text":
			out.Content += c.Text
		case "tool_use":
			args, _ := json.Marshal(c.Input)
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID: c.ID, Name: c.Name, Arguments: string(args),
			})
		}
	}
	return out, nil
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
