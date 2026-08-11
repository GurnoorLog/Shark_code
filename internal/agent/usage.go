package agent

import (
	"strings"
)

// Context window sizes (tokens) per provider/model, used for the
// "context used" indicator.
var contextWindows = []struct {
	match   string
	window  int
}{
	{"gpt-4.1", 1_047_576},
	{"gpt-4o", 128_000},
	{"gpt-4", 128_000},
	{"claude", 200_000},
	{"gemini-2.5", 1_048_576},
	{"gemini-2.0", 1_048_576},
	{"gemini", 1_048_576},
	{"qwen2.5:7b", 32_768},
	{"qwen2.5", 32_768},
	{"llama-3.2", 128_000},
	{"llama-3", 128_000},
	{"gemma-2", 8_192},
	{"gemma-4", 256_000},
	{"phi3", 128_000},
	{"minimax", 1_000_000},
	{"kimi", 262_144},
	{"fireworks", 131_072},
}

// contextWindowFor returns the best-known context size for a model.
func contextWindowFor(model string) int {
	for _, cw := range contextWindows {
		if strings.Contains(model, cw.match) {
			return cw.window
		}
	}
	return 128_000
}

// Per-1M-token prices (input, output) in USD for cost estimation.
var pricing = []struct {
	match   string
	input   float64
	output  float64
}{
	{"gpt-4o-mini", 0.15, 0.60},
	{"gpt-4o", 2.50, 10.00},
	{"gpt-4.1", 2.00, 8.00},
	{"gpt-4", 30.00, 60.00},
	{"claude-opus", 15.00, 75.00},
	{"claude-sonnet", 3.00, 15.00},
	{"claude-haiku", 0.25, 1.25},
	{"claude", 3.00, 15.00},
	{"gemini-2.5-flash", 0.30, 2.50},
	{"gemini-2.5-pro", 1.25, 10.00},
	{"gemini-2.0-flash", 0.10, 0.40},
	{"gemini-2.0-pro", 1.25, 10.00},
	{"gemini", 1.25, 10.00},
	{"minimax", 0.80, 3.20},
	{"kimi", 0.50, 2.50},
	{"gemma", 0.20, 0.20},
	{"qwen", 0.20, 0.20},
	{"llama", 0.20, 0.20},
	{"phi3", 0.20, 0.20},
	{"local", 0, 0},
	{"fireworks", 0.50, 2.00},
}

// estimateCostUSD estimates the cost of the given token usage.
func estimateCostUSD(providerName, model string, input, output int) float64 {
	if strings.Contains(providerName, "local") {
		return 0
	}
	pin, pout := 0.50, 2.00 // fallback
	for _, p := range pricing {
		if strings.Contains(model, p.match) {
			pin, pout = p.input, p.output
			break
		}
	}
	return float64(input)/1e6*pin + float64(output)/1e6*pout
}

// Usage holds cumulative token usage across a session.
type Usage struct {
	InputTokens  int
	OutputTokens int
	CostUSD      float64
}

func (u *Usage) Total() int { return u.InputTokens + u.OutputTokens }
