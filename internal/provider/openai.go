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

// OpenAICompatible implements the OpenAI Chat Completions protocol,
// covering OpenAI, Fireworks, and local llama-server.
type OpenAICompatible struct {
	name    string
	cfg     *config.ProviderConfig
	client  *http.Client
	apiBase string
}

func NewOpenAICompatible(name string, cfg *config.ProviderConfig) *OpenAICompatible {
	base := cfg.BaseURL
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	return &OpenAICompatible{
		name:    name,
		cfg:     cfg,
		client:  &http.Client{Timeout: 180 * time.Second},
		apiBase: base,
	}
}

func (p *OpenAICompatible) Name() string { return p.name }

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

type oaiMsg struct {
	Role         string         `json:"role"`
	Content      *string        `json:"content,omitempty"`
	ToolCallID   string         `json:"tool_call_id,omitempty"`
	ToolCalls    []oaiToolCall  `json:"tool_calls,omitempty"`
	Name         string         `json:"name,omitempty"`
}

type oaiReq struct {
	Model    string    `json:"model"`
	Messages []oaiMsg  `json:"messages"`
	Tools    []oaiTool `json:"tools,omitempty"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		Parameters  map[string]any `json:"parameters"`
	} `json:"function"`
}

type oaiResp struct {
	Choices []struct {
		Message struct {
			Role      string        `json:"role"`
			Content   *string       `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		TotalTokens int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *OpenAICompatible) Chat(ctx context.Context, msgs []Message, tools []ToolDef) (*Response, error) {
	var oaiMsgs []oaiMsg
	for _, m := range msgs {
		om := oaiMsg{Role: string(m.Role)}
		content := m.Content
		om.Content = &content
		om.ToolCallID = m.ToolCallID
		for _, tc := range m.ToolCalls {
			om.ToolCalls = append(om.ToolCalls, oaiToolCall{
				ID:   tc.ID,
				Type: "function",
				Function: struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}{Name: tc.Name, Arguments: json.RawMessage(tc.Arguments)},
			})
		}
		oaiMsgs = append(oaiMsgs, om)
	}

	var oaiTools []oaiTool
	for _, t := range tools {
		ot := oaiTool{Type: "function"}
		ot.Function.Name = t.Name
		ot.Function.Description = t.Description
		ot.Function.Parameters = t.Parameters
		oaiTools = append(oaiTools, ot)
	}

	body, _ := json.Marshal(oaiReq{
		Model:    p.cfg.Model,
		Messages: oaiMsgs,
		Tools:    oaiTools,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", p.apiBase+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}

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
		return nil, fmt.Errorf("%s: %d %s", p.name, resp.StatusCode, string(raw))
	}

	var oa oaiResp
	if err := json.Unmarshal(raw, &oa); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if len(oa.Choices) == 0 {
		return nil, fmt.Errorf("no choices")
	}
	ch := oa.Choices[0]
	out := &Response{
		FinishReason: ch.FinishReason,
		Tokens:       oa.Usage.TotalTokens,
	}
	if ch.Message.Content != nil {
		out.Content = *ch.Message.Content
	}
	for _, tc := range ch.Message.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: string(tc.Function.Arguments),
		})
	}
	return out, nil
}
