package onboarding

import (
	"fmt"
	"strings"

	"github.com/codeforge/tui/internal/config"
)

// ProviderOrder is the default bootstrap priority when several keys exist
// and the user has not set default_provider / onboarding preference.
// Grok (xAI) first — CodeForge Grok-parity default.
var ProviderOrder = []string{"grok", "gemini", "claude", "openai", "ollama"}

// ProviderMeta is display metadata for wizards and /setup.
type ProviderMeta struct {
	Name         string
	Title        string
	EnvPrimary   string
	KeyHint      string
	DocsURL      string
	DefaultModel string
}

// Catalog is the user-facing provider list (stable order).
var Catalog = []ProviderMeta{
	{Name: "grok", Title: "Grok 4.5 (xAI)", EnvPrimary: "XAI_API_KEY", KeyHint: "xai-…", DocsURL: "https://console.x.ai/", DefaultModel: "grok-4.5"},
	{Name: "gemini", Title: "Gemini (Google)", EnvPrimary: "GEMINI_API_KEY", KeyHint: "AIza…", DocsURL: "https://aistudio.google.com/apikey", DefaultModel: "gemini-2.5-flash"},
	{Name: "claude", Title: "Claude (Anthropic)", EnvPrimary: "ANTHROPIC_API_KEY", KeyHint: "sk-ant-…", DocsURL: "https://console.anthropic.com/", DefaultModel: "claude-sonnet-4-20250514"},
	{Name: "openai", Title: "OpenAI-compatible", EnvPrimary: "OPENAI_API_KEY", KeyHint: "sk-…", DocsURL: "https://platform.openai.com/api-keys", DefaultModel: "gpt-4o-mini"},
	{Name: "ollama", Title: "Ollama (local)", EnvPrimary: "OLLAMA_HOST", KeyHint: "(no cloud key)", DocsURL: "https://ollama.com/", DefaultModel: "llama3.2"},
}

// KeyPresence is one provider's detection result.
type KeyPresence struct {
	Name    string
	Present bool
	Source  string // env:XAI_API_KEY | config | local | missing
	Title   string
}

// DetectAll returns key presence for every catalog provider.
func DetectAll() []KeyPresence {
	out := make([]KeyPresence, 0, len(Catalog))
	for _, m := range Catalog {
		src, ok := KeySource(m.Name)
		// Ollama: present only if reachable is too heavy here; "local" always listed
		if m.Name == "ollama" {
			src, ok = "local (optional)", true
			// don't count ollama as "has key" for multi-key unless registered later
			ok = false
			src = "local (start ollama serve)"
		}
		out = append(out, KeyPresence{Name: m.Name, Present: ok, Source: src, Title: m.Title})
	}
	// fix ollama presence: KeySource returns local,true — for DetectAll cloud keys we want
	// ollama separate. Re-read:
	for i := range out {
		if out[i].Name == "ollama" {
			// optional local — not a cloud API key
			out[i].Present = false
			out[i].Source = "local (optional)"
		}
	}
	return out
}

// PresentCloudKeys returns providers that have env/config API keys (excludes ollama).
func PresentCloudKeys() []KeyPresence {
	var out []KeyPresence
	for _, k := range DetectAll() {
		if k.Name == "ollama" {
			continue
		}
		src, ok := KeySource(k.Name)
		if ok {
			out = append(out, KeyPresence{Name: k.Name, Present: true, Source: src, Title: k.Title})
		}
	}
	return out
}

// CountPresentKeys counts cloud API keys available.
func CountPresentKeys() int {
	return len(PresentCloudKeys())
}

// Resolution explains which provider is/should be active and why.
type Resolution struct {
	Provider string
	Model    string
	Reason   string // human-readable
	Source   string // key source for that provider
	// Alternatives lists other providers that also have keys.
	Alternatives []string
}

// ResolveActive picks the active provider with an explicit reason.
// Priority:
//  1. onboarding.json preferred provider (if key still present)
//  2. config default_provider (if key present)
//  3. ProviderOrder among present keys (grok → gemini → claude → openai)
//  4. empty if nothing available
func ResolveActive(cfg *config.Config) Resolution {
	present := PresentCloudKeys()
	have := map[string]KeyPresence{}
	for _, p := range present {
		have[p.Name] = p
	}
	var alts []string
	for _, p := range present {
		alts = append(alts, p.Name)
	}

	// 1) onboarding preference
	if st, err := Load(); err == nil && st.Provider != "" {
		name := normalizeName(st.Provider)
		if name == "ollama" || have[name].Present {
			src, _ := KeySource(name)
			return Resolution{
				Provider: name, Model: firstNonEmpty(st.Model, DefaultModels[name]),
				Reason: fmt.Sprintf("onboarding preference (%s)", name),
				Source: src, Alternatives: others(name, alts),
			}
		}
	}

	// 2) config default_provider
	if cfg != nil {
		def := normalizeName(cfg.DefaultProvider)
		if def != "" && (def == "ollama" || have[def].Present) {
			src, _ := KeySource(def)
			model := DefaultModels[def]
			if cfg.Providers != nil {
				if p, ok := cfg.Providers[def]; ok && p.DefaultModel != "" {
					model = p.DefaultModel
				}
			}
			return Resolution{
				Provider: def, Model: model,
				Reason: fmt.Sprintf("config default_provider=%s", def),
				Source: src, Alternatives: others(def, alts),
			}
		}
	}

	// 3) fixed priority among present keys
	for _, name := range ProviderOrder {
		if name == "ollama" {
			continue
		}
		if p, ok := have[name]; ok && p.Present {
			model := DefaultModels[name]
			if cfg != nil && cfg.Providers != nil {
				if cp, ok := cfg.Providers[name]; ok && cp.DefaultModel != "" {
					model = cp.DefaultModel
				}
			}
			return Resolution{
				Provider: name, Model: model,
				Reason: fmt.Sprintf("priority order (first available): %s — %s", name, p.Source),
				Source: p.Source, Alternatives: others(name, alts),
			}
		}
	}

	return Resolution{
		Provider: "", Reason: "no API key found", Source: "missing", Alternatives: nil,
	}
}

// FormatStatus is the multi-line multi-provider status for /provider /setup /welcome.
func FormatStatus(cfg *config.Config, activeName string) string {
	var b strings.Builder
	b.WriteString("Providers & API keys\n")
	b.WriteString("────────────────────\n")
	res := ResolveActive(cfg)
	for _, m := range Catalog {
		src, ok := KeySource(m.Name)
		if m.Name == "ollama" {
			src = "local (optional)"
			ok = false
		}
		mark := "○"
		if ok || m.Name == "ollama" {
			if ok {
				mark = "✓"
			} else {
				mark = "·"
			}
		}
		active := ""
		if activeName != "" && normalizeName(activeName) == m.Name {
			active = "  ← active"
		} else if activeName == "" && res.Provider == m.Name {
			active = "  ← will activate"
		}
		b.WriteString(fmt.Sprintf("  %s %-8s  %-22s  %s%s\n", mark, m.Name, m.Title, src, active))
	}
	b.WriteString("\n")
	if res.Provider != "" {
		b.WriteString(fmt.Sprintf("Active selection: %s\n", res.Provider))
		b.WriteString(fmt.Sprintf("  why:  %s\n", res.Reason))
		b.WriteString(fmt.Sprintf("  key:  %s\n", res.Source))
		if len(res.Alternatives) > 0 {
			b.WriteString(fmt.Sprintf("  also: %s  → switch with /provider <name>\n", strings.Join(res.Alternatives, ", ")))
		}
	} else {
		b.WriteString("⚠ No cloud API key detected.\n")
		b.WriteString("  → /setup <provider> <api-key>   or set env (XAI_API_KEY / GEMINI_API_KEY / …)\n")
	}
	b.WriteString("\nPriority if several keys & no preference: grok → gemini → claude → openai\n")
	b.WriteString("Set preference: /provider <name>   ·  re-run guide: /setup\n")
	return b.String()
}

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "xai" {
		return "grok"
	}
	if s == "anthropic" {
		return "claude"
	}
	return s
}

func others(active string, all []string) []string {
	var out []string
	for _, n := range all {
		if n != active {
			out = append(out, n)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// MetaFor returns catalog entry or a minimal fallback.
func MetaFor(name string) ProviderMeta {
	name = normalizeName(name)
	for _, m := range Catalog {
		if m.Name == name {
			return m
		}
	}
	return ProviderMeta{Name: name, Title: name, DefaultModel: DefaultModels[name]}
}
