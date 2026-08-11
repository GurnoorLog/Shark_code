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

// Gemini implements the Gemini generateContent API.
type Gemini struct {
	cfg    *config.ProviderConfig
	client *http.Client
}

func NewGemini(cfg *config.ProviderConfig) *Gemini {
	return &Gemini{
		cfg:    cfg,
		client: &http.Client{Timeout: 180 * time.Second},
	}
}

func (p *Gemini) Name() string { return "gemini" }

func (p *Gemini) Model() string { return p.cfg.Model }

type gemPart struct {
	Text            string            `json:"text,omitempty"`
	FunctionCall    *gemFuncCall      `json:"functionCall,omitempty"`
	FunctionResponse *gemFuncResponse `json:"functionResponse,omitempty"`
}

type gemFuncCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

type gemFuncResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type gemContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []gemPart  `json:"parts"`
}

type gemTool struct {
	FunctionDeclarations []map[string]any `json:"functionDeclarations"`
}

type gemReq struct {
	Contents          []gemContent `json:"contents"`
	SystemInstruction *gemContent  `json:"systemInstruction,omitempty"`
	Tools             []gemTool    `json:"tools,omitempty"`
}

type gemResp struct {
	Candidates []struct {
		Content gemContent `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		TotalTokenCount int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (p *Gemini) Chat(ctx context.Context, msgs []Message, tools []ToolDef) (*Response, error) {
	var contents []gemContent
	var systemText string

	for _, m := range msgs {
		switch m.Role {
		case RoleSystem:
			systemText += m.Content + "\n"
		case RoleUser:
			if m.ToolCallID != "" {
				name := m.ToolName
				if name == "" {
					name = "tool"
				}
				contents = append(contents, gemContent{Role: "user", Parts: []gemPart{{
					FunctionResponse: &gemFuncResponse{
						Name:     name,
						Response: map[string]any{"result": m.Content},
					},
				}}})
			} else {
				contents = append(contents, gemContent{Role: "user", Parts: []gemPart{{Text: m.Content}}})
			}
		case RoleAssistant:
			var parts []gemPart
			if m.Content != "" {
				parts = append(parts, gemPart{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				var args map[string]any
				json.Unmarshal([]byte(tc.Arguments), &args)
				parts = append(parts, gemPart{FunctionCall: &gemFuncCall{Name: tc.Name, Args: args}})
			}
			contents = append(contents, gemContent{Role: "model", Parts: parts})
		}
	}

	var gemTools []gemTool
	if len(tools) > 0 {
		var decls []map[string]any
		for _, t := range tools {
			decls = append(decls, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  t.Parameters,
			})
		}
		gemTools = append(gemTools, gemTool{FunctionDeclarations: decls})
	}

	reqBody := gemReq{Contents: contents, Tools: gemTools}
	if systemText != "" {
		reqBody.SystemInstruction = &gemContent{Parts: []gemPart{{Text: systemText}}}
	}

	body, _ := json.Marshal(reqBody)

	url := p.cfg.BaseURL + "/models/" + p.cfg.Model + ":generateContent?key=" + p.cfg.APIKey
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("gemini: %d %s", resp.StatusCode, string(raw))
	}

	var gr gemResp
	if err := json.Unmarshal(raw, &gr); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	if len(gr.Candidates) == 0 {
		return nil, fmt.Errorf("gemini: no candidates")
	}

	out := &Response{Tokens: gr.UsageMetadata.TotalTokenCount, InputTokens: gr.UsageMetadata.TotalTokenCount, OutputTokens: 0}
	for _, part := range gr.Candidates[0].Content.Parts {
		if part.Text != "" {
			out.Content += part.Text
		}
		if part.FunctionCall != nil {
			args, _ := json.Marshal(part.FunctionCall.Args)
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID:   part.FunctionCall.Name + "_" + randID(),
				Name: part.FunctionCall.Name,
				Arguments: string(args),
			})
		}
	}
	return out, nil
}
