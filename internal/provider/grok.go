package provider

import (
	"os"
	"strings"
)

// NewGrokProvider creates an xAI Grok provider (OpenAI-compatible Chat Completions).
// Default model: grok-4.5. Auth: XAI_API_KEY or GROK_API_KEY.
func NewGrokProvider(apiKey, defaultModel string) *OpenAIProvider {
	if apiKey == "" {
		apiKey = os.Getenv("XAI_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GROK_API_KEY")
	}
	if defaultModel == "" {
		defaultModel = os.Getenv("GROK_MODEL")
	}
	if defaultModel == "" {
		defaultModel = "grok-4.5"
	}
	endpoint := os.Getenv("XAI_BASE_URL")
	if endpoint == "" {
		endpoint = os.Getenv("GROK_BASE_URL")
	}
	if endpoint == "" {
		endpoint = "https://api.x.ai/v1"
	}
	endpoint = strings.TrimRight(endpoint, "/")

	return &OpenAIProvider{
		apiKey:   apiKey,
		model:    defaultModel,
		endpoint: endpoint,
		client:   defaultHTTPClient(),
		name:     "grok",
		models:   GrokModels(),
	}
}

// GrokModels is the catalog for UI / cost estimation.
func GrokModels() []ModelInfo {
	return []ModelInfo{
		{ID: "grok-4.5", Name: "Grok 4.5", ContextWindow: 500000, InputCost: 2.0, OutputCost: 6.0},
		{ID: "grok-4", Name: "Grok 4", ContextWindow: 256000, InputCost: 3.0, OutputCost: 15.0},
		{ID: "grok-3", Name: "Grok 3", ContextWindow: 131072, InputCost: 3.0, OutputCost: 15.0},
		{ID: "grok-3-mini", Name: "Grok 3 Mini", ContextWindow: 131072, InputCost: 0.30, OutputCost: 0.50},
		{ID: "grok-2-latest", Name: "Grok 2", ContextWindow: 131072, InputCost: 2.0, OutputCost: 10.0},
	}
}
