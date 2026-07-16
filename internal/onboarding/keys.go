package onboarding

import (
	"fmt"
	"os"
	"strings"

	"github.com/codeforge/tui/internal/config"
)

// EnvKeys maps provider name → env var names (first non-empty wins).
var EnvKeys = map[string][]string{
	"grok":   {"XAI_API_KEY", "GROK_API_KEY"},
	"xai":    {"XAI_API_KEY", "GROK_API_KEY"},
	"gemini": {"GEMINI_API_KEY"},
	"claude": {"ANTHROPIC_API_KEY"},
	"openai": {"OPENAI_API_KEY"},
}

// DefaultModels per provider for wizard defaults.
var DefaultModels = map[string]string{
	"grok":   "grok-4.5",
	"gemini": "gemini-2.5-flash",
	"claude": "claude-sonnet-4-20250514",
	"openai": "gpt-4o-mini",
	"ollama": "llama3.2",
}

// DetectProviderFromKey guesses provider from key prefix / shape.
func DetectProviderFromKey(key string) string {
	k := strings.TrimSpace(key)
	switch {
	case strings.HasPrefix(k, "xai-"), strings.HasPrefix(k, "xai_"):
		return "grok"
	case strings.HasPrefix(k, "sk-ant-"):
		return "claude"
	case strings.HasPrefix(k, "AIza"):
		return "gemini"
	case strings.HasPrefix(k, "sk-"):
		return "openai"
	default:
		return ""
	}
}

// HasAnyAPIKey is true if env or config has at least one cloud provider key.
func HasAnyAPIKey() bool {
	return CountPresentKeys() > 0
}

// KeySource describes where a provider's key comes from.
// source examples: "env:XAI_API_KEY", "config", "missing"
func KeySource(provider string) (source string, present bool) {
	provider = normalizeName(provider)
	if provider == "ollama" {
		return "local", true
	}
	envs := EnvKeys[provider]
	for _, e := range envs {
		if os.Getenv(e) != "" {
			return "env:" + e, true
		}
	}
	cfg, err := config.Load()
	if err == nil && cfg != nil {
		if p, ok := cfg.Providers[provider]; ok && strings.TrimSpace(p.APIKey) != "" {
			return "config", true
		}
		if provider == "grok" {
			if p, ok := cfg.Providers["xai"]; ok && strings.TrimSpace(p.APIKey) != "" {
				return "config", true
			}
		}
	}
	return "missing", false
}

// FormatKeySources returns a multi-line summary for /provider and /setup.
// Prefer FormatStatus when config is available (shows active reason).
func FormatKeySources() string {
	return FormatStatus(nil, "")
}

// FormatKeySourcesWithActive includes the currently selected registry name.
func FormatKeySourcesWithActive(cfg *config.Config, active string) string {
	return FormatStatus(cfg, active)
}

// EnvNameForProvider returns the preferred env var name for docs/hints.
func EnvNameForProvider(provider string) string {
	envs := EnvKeys[normalizeName(provider)]
	if len(envs) == 0 {
		return ""
	}
	return envs[0]
}

// MaskKey returns a short redacted form for UI (never full secret).
func MaskKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) <= 8 {
		return "••••"
	}
	return key[:4] + "…" + key[len(key)-4:]
}

// ExplainPriority is a short one-liner for docs/banner.
func ExplainPriority() string {
	return fmt.Sprintf("Priority: %s (or config default_provider / onboarding preference)",
		strings.Join(ProviderOrder[:4], " → "))
}
