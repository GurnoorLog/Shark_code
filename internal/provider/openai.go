package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

func (p *OpenAICompatible) Model() string { return p.cfg.Model }

type oaiToolCall struct {
	ID       string          `json:"id"`
	Index    int             `json:"index"`
	Type     string          `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// oaiStreamChunk is one SSE `data:` line in a streaming response.
type oaiStreamChunk struct {
	Choices []struct {
		Delta struct {
			Content   *string       `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
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
	Stream   bool      `json:"stream,omitempty"`
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
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (p *OpenAICompatible) Chat(ctx context.Context, msgs []Message, tools []ToolDef) (*Response, error) {
	oaiMsgs, oaiTools := buildOAI(msgs, tools)
	body, _ := json.Marshal(oaiReq{
		Model:    p.cfg.Model,
		Messages: oaiMsgs,
		Tools:    oaiTools,
	})
	raw, err := p.post(ctx, body)
	if err != nil {
		return nil, err
	}
	return parseOAIResp(raw)
}

// ChatStream streams the reply text token-by-token through onDelta.
func (p *OpenAICompatible) ChatStream(ctx context.Context, msgs []Message, tools []ToolDef, onDelta func(string)) (*Response, error) {
	oaiMsgs, oaiTools := buildOAI(msgs, tools)
	body, _ := json.Marshal(oaiReq{
		Model:    p.cfg.Model,
		Messages: oaiMsgs,
		Tools:    oaiTools,
		Stream:   true,
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

	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s: %d %s", p.name, resp.StatusCode, string(raw))
	}

	// Accumulate a non-streaming-style response for the caller.
	out := &Response{}
	var toolCalls []oaiToolCall
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}
		var chunk oaiStreamChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}
		if chunk.Usage.PromptTokens != 0 || chunk.Usage.CompletionTokens != 0 || chunk.Usage.TotalTokens != 0 {
			out.InputTokens = chunk.Usage.PromptTokens
			out.OutputTokens = chunk.Usage.CompletionTokens
			out.Tokens = chunk.Usage.TotalTokens
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		ch := chunk.Choices[0]
		if ch.Delta.Content != nil {
			out.Content += *ch.Delta.Content
			if onDelta != nil {
				onDelta(*ch.Delta.Content)
			}
		}
		for _, tc := range ch.Delta.ToolCalls {
			appendToolCall(&toolCalls, tc)
		}
		if ch.FinishReason != "" {
			out.FinishReason = ch.FinishReason
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	for _, tc := range toolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: string(tc.Function.Arguments),
		})
	}
	return out, nil
}

func buildOAI(msgs []Message, tools []ToolDef) ([]oaiMsg, []oaiTool) {
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
	return oaiMsgs, oaiTools
}

func (p *OpenAICompatible) post(ctx context.Context, body []byte) ([]byte, error) {
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
	return raw, nil
}

// appendToolCall assembles OpenAI streamed tool-call deltas, which arrive
// across multiple chunks with a shared index.
func appendToolCall(acc *[]oaiToolCall, tc oaiToolCall) {
	for i := range *acc {
		if (*acc)[i].Index == tc.Index {
			(*acc)[i].Function.Name += tc.Function.Name
			(*acc)[i].Function.Arguments = append((*acc)[i].Function.Arguments, tc.Function.Arguments...)
			if (*acc)[i].ID == "" {
				(*acc)[i].ID = tc.ID
			}
			return
		}
	}
	*acc = append(*acc, tc)
}

func parseOAIResp(raw []byte) (*Response, error) {
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
		InputTokens:  oa.Usage.PromptTokens,
		OutputTokens: oa.Usage.CompletionTokens,
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
