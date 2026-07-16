package onboarding

import (
	"fmt"
	"os"
	"strings"

	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/provider"
)

// ApplyKey registers (or replaces) a provider with the given API key on the registry,
// sets process env for the session, and persists key into config.yaml.
// Also sets default_provider + onboarding preference so multi-key choice sticks.
func ApplyKey(reg *provider.Registry, name, apiKey, model string) (provider.Provider, error) {
	name = normalizeName(name)
	apiKey = strings.TrimSpace(apiKey)
	if name == "" {
		return nil, fmt.Errorf("provider name required")
	}
	if name == "ollama" {
		p := provider.NewOllamaProvider(model)
		if err := p.ValidateConfig(); err != nil {
			return nil, err
		}
		_ = reg.Register(p)
		_ = reg.Switch("ollama")
		return p, nil
	}
	if apiKey == "" {
		return nil, fmt.Errorf("API key required for %s", name)
	}
	if model == "" {
		model = DefaultModels[name]
	}

	var p provider.Provider
	switch name {
	case "grok":
		_ = os.Setenv("XAI_API_KEY", apiKey)
		p = provider.NewGrokProvider(apiKey, model)
	case "gemini":
		_ = os.Setenv("GEMINI_API_KEY", apiKey)
		p = provider.NewGeminiProvider(apiKey, model)
	case "claude":
		_ = os.Setenv("ANTHROPIC_API_KEY", apiKey)
		p = provider.NewClaudeProvider(apiKey, model)
	case "openai":
		_ = os.Setenv("OPENAI_API_KEY", apiKey)
		p = provider.NewOpenAIProvider(apiKey, model)
	default:
		return nil, fmt.Errorf("unknown provider %q (grok|gemini|claude|openai|ollama)", name)
	}
	if err := p.ValidateConfig(); err != nil {
		return nil, err
	}
	_ = reg.Register(p)
	_ = reg.Switch(name)
	_ = config.SaveProviderKey(name, apiKey, model)
	_ = config.SaveDefaultProvider(name)
	_ = MarkCompleted(name, model)
	return p, nil
}

// ProviderHealthy is true when current provider validates.
func ProviderHealthy(reg *provider.Registry) bool {
	if reg == nil {
		return false
	}
	p, err := reg.Current()
	if err != nil {
		return false
	}
	// bare Claude placeholder without key is not healthy
	if err := p.ValidateConfig(); err != nil {
		return false
	}
	return true
}
