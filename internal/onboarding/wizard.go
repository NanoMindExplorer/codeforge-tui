package onboarding

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/provider"
)

// WizardOptions control the CLI first-run flow.
type WizardOptions struct {
	In  io.Reader
	Out io.Writer
	// Registry is optional; when set, successful keys are registered immediately.
	Registry *provider.Registry
	// Config optional — used for default_provider display.
	Config *config.Config
	// SkipValidation skips live ValidateConfig (tests).
	SkipValidation bool
}

// RunWizard is the interactive first-run setup with multi-provider clarity.
func RunWizard(opt WizardOptions) error {
	in := opt.In
	if in == nil {
		in = os.Stdin
	}
	out := opt.Out
	if out == nil {
		out = os.Stdout
	}
	r := bufio.NewReader(in)
	cfg := opt.Config
	if cfg == nil {
		cfg, _ = config.Load()
	}

	// Start screen: CodeForge + small By NanoMindExplorer
	WriteBrandStart(out, true)
	fmt.Fprintln(out, "  First-run setup  ·  multi-provider")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "You need one API key. Several providers can coexist;")
	fmt.Fprintln(out, "only ONE is active at a time (footer shows which).")
	fmt.Fprintln(out)
	fmt.Fprintln(out, ExplainPriority())
	fmt.Fprintln(out)

	present := PresentCloudKeys()
	fmt.Fprintln(out, "① Detected keys")
	printDetected(out, present)

	// ── Path A: multiple keys already present → pick default (no re-paste) ──
	if len(present) > 1 {
		return wizardPickAmongPresent(opt, out, r, cfg, present)
	}

	// ── Path B: exactly one key → confirm or switch ──
	if len(present) == 1 {
		return wizardConfirmSingle(opt, out, r, cfg, present[0])
	}

	// ── Path C: no keys → choose provider + paste ──
	return wizardAddNewKey(opt, out, r)
}

func wizardPickAmongPresent(opt WizardOptions, out io.Writer, r *bufio.Reader, cfg *config.Config, present []KeyPresence) error {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "② You have %d providers with keys. Pick the DEFAULT active one:\n", len(present))
	for i, p := range present {
		fmt.Fprintf(out, "   [%d] %-8s  %s\n", i+1, p.Name, p.Source)
	}
	res := ResolveActive(cfg)
	fmt.Fprintf(out, "   Suggested: %s (%s)\n", res.Provider, res.Reason)
	fmt.Fprint(out, "   > ")
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "s" || line == "skip" || line == "q" {
		// accept suggestion
		if res.Provider != "" {
			return finalizePreference(opt, out, res.Provider, res.Model)
		}
		_ = MarkSkipped()
		fmt.Fprintln(out, "   Skipped. Use /provider later.")
		return nil
	}
	name := mapChoice(line)
	if name == "" {
		// numeric index
		if n := atoi(line); n >= 1 && n <= len(present) {
			name = present[n-1].Name
		} else {
			name = normalizeName(line)
		}
	}
	ok := false
	for _, p := range present {
		if p.Name == name {
			ok = true
			break
		}
	}
	if !ok {
		fmt.Fprintf(out, "   ⚠ %q is not among detected keys. Try again with /setup.\n", name)
		_ = MarkSkipped()
		return nil
	}
	model := DefaultModels[name]
	if cfg != nil && cfg.Providers != nil {
		if p, ok := cfg.Providers[name]; ok && p.DefaultModel != "" {
			model = p.DefaultModel
		}
	}
	fmt.Fprintf(out, "\n③ Model for %s [%s] (Enter to keep):\n", name, model)
	fmt.Fprint(out, "   model> ")
	mline, _ := r.ReadString('\n')
	if strings.TrimSpace(mline) != "" {
		model = strings.TrimSpace(mline)
	}
	return finalizePreference(opt, out, name, model)
}

func wizardConfirmSingle(opt WizardOptions, out io.Writer, r *bufio.Reader, cfg *config.Config, only KeyPresence) error {
	fmt.Fprintln(out)
	fmt.Fprintf(out, "② Only one key found: %s (%s)\n", only.Name, only.Source)
	fmt.Fprintln(out, "   [Enter] use it   [a] add another key   [s] skip")
	fmt.Fprint(out, "   > ")
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "s" || line == "skip" {
		_ = MarkSkipped()
		fmt.Fprintln(out, "   Skipped.")
		return nil
	}
	if line == "a" || line == "add" {
		return wizardAddNewKey(opt, out, r)
	}
	// enter or anything else → use detected
	model := DefaultModels[only.Name]
	if cfg != nil && cfg.Providers != nil {
		if p, ok := cfg.Providers[only.Name]; ok && p.DefaultModel != "" {
			model = p.DefaultModel
		}
	}
	return finalizePreference(opt, out, only.Name, model)
}

func wizardAddNewKey(opt WizardOptions, out io.Writer, r *bufio.Reader) error {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "② Choose provider to configure:")
	for i, m := range Catalog {
		fmt.Fprintf(out, "   [%d] %-8s  %s\n", i+1, m.Name, m.Title)
		if m.Name != "ollama" {
			fmt.Fprintf(out, "         env %s  ·  key looks like %s\n", m.EnvPrimary, m.KeyHint)
			fmt.Fprintf(out, "         %s\n", m.DocsURL)
		}
	}
	fmt.Fprintln(out, "   [s] skip for now")
	fmt.Fprint(out, "   > ")
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "s" || line == "skip" || line == "q" {
		_ = MarkSkipped()
		fmt.Fprintln(out, "   Skipped. Run /setup inside the TUI or set an env key.")
		return nil
	}

	name := mapChoice(line)
	if name == "" {
		name = normalizeName(line)
		if name != "grok" && name != "gemini" && name != "claude" && name != "openai" && name != "ollama" {
			if det := DetectProviderFromKey(line); det != "" {
				return finishWithKey(opt, out, r, det, line)
			}
			fmt.Fprintln(out, "   Unknown choice — try /setup in the TUI later.")
			_ = MarkSkipped()
			return nil
		}
	}

	if name == "ollama" {
		if opt.Registry != nil {
			p, err := ApplyKey(opt.Registry, "ollama", "", "")
			if err != nil {
				fmt.Fprintf(out, "   ⚠ Ollama: %v\n   Start `ollama serve` and pull a model, then /setup.\n", err)
				return nil
			}
			fmt.Fprintf(out, "   ✓ Ollama · model %s\n", p.Model())
		} else {
			_ = MarkCompleted("ollama", DefaultModels["ollama"])
			_ = config.SaveDefaultProvider("ollama")
			fmt.Fprintln(out, "   ✓ Ollama selected (ensure `ollama serve` is running)")
		}
		printDone(out, "ollama")
		return nil
	}

	meta := MetaFor(name)
	fmt.Fprintf(out, "\n③ Paste API key for %s\n", meta.Title)
	fmt.Fprintf(out, "   env: %s   shape: %s\n", meta.EnvPrimary, meta.KeyHint)
	fmt.Fprintf(out, "   get key: %s\n", meta.DocsURL)
	fmt.Fprint(out, "   key> ")
	key, _ := r.ReadString('\n')
	key = strings.TrimSpace(key)
	if key == "" {
		fmt.Fprintln(out, "   No key entered. You can /setup later.")
		_ = MarkSkipped()
		return nil
	}
	if det := DetectProviderFromKey(key); det != "" && det != name {
		fmt.Fprintf(out, "   ℹ key prefix looks like %s — using that provider\n", det)
		name = det
	}
	fmt.Fprintf(out, "   (saved as %s)\n", MaskKey(key))
	return finishWithKey(opt, out, r, name, key)
}

func finalizePreference(opt WizardOptions, out io.Writer, name, model string) error {
	name = normalizeName(name)
	_ = config.SaveDefaultProvider(name)
	_ = MarkCompleted(name, model)
	if opt.Registry != nil {
		// Switch if already registered; else re-apply from env/config
		if err := opt.Registry.Switch(name); err != nil {
			// try construct from env
			if _, err2 := ApplyKey(opt.Registry, name, envKeyValue(name), model); err2 != nil {
				fmt.Fprintf(out, "   ⚠ registered switch: %v\n", err)
			}
		} else if model != "" {
			if p, err := opt.Registry.Current(); err == nil {
				_ = p.SetModel(model)
			}
		}
	}
	src, _ := KeySource(name)
	fmt.Fprintf(out, "\n   ✓ Default provider: %s\n", name)
	fmt.Fprintf(out, "     model: %s\n", model)
	fmt.Fprintf(out, "     key:   %s\n", src)
	printDone(out, name)
	return nil
}

func finishWithKey(opt WizardOptions, out io.Writer, r *bufio.Reader, name, key string) error {
	model := DefaultModels[name]
	fmt.Fprintf(out, "\n④ Default model [%s] (Enter to keep, or type id):\n", model)
	fmt.Fprint(out, "   model> ")
	mline, _ := r.ReadString('\n')
	mline = strings.TrimSpace(mline)
	if mline != "" {
		model = mline
	}

	reg := opt.Registry
	if reg == nil {
		reg = provider.NewRegistry()
	}
	p, err := ApplyKey(reg, name, key, model)
	if err != nil {
		fmt.Fprintf(out, "   ⚠ %v\n   Key not saved. Fix and run /setup.\n", err)
		return nil
	}
	if !opt.SkipValidation {
		if err := p.ValidateConfig(); err != nil {
			fmt.Fprintf(out, "   ⚠ validate: %v\n", err)
			return nil
		}
	}
	fmt.Fprintf(out, "   ✓ %s ready · model %s\n", name, p.Model())
	fmt.Fprintf(out, "   key source: %s\n", mustSource(name))
	printDone(out, name)
	return nil
}

func envKeyValue(name string) string {
	for _, e := range EnvKeys[normalizeName(name)] {
		if v := os.Getenv(e); v != "" {
			return v
		}
	}
	return ""
}

func printDetected(out io.Writer, present []KeyPresence) {
	if len(present) == 0 {
		fmt.Fprintln(out, "   (none yet)")
		for _, m := range Catalog {
			if m.Name == "ollama" {
				continue
			}
			fmt.Fprintf(out, "   ○ %-8s  set %s\n", m.Name, m.EnvPrimary)
		}
		return
	}
	for _, p := range present {
		fmt.Fprintf(out, "   ✓ %-8s  %s\n", p.Name, p.Source)
	}
	// also show missing
	have := map[string]bool{}
	for _, p := range present {
		have[p.Name] = true
	}
	for _, m := range Catalog {
		if m.Name == "ollama" || have[m.Name] {
			continue
		}
		fmt.Fprintf(out, "   ○ %-8s  missing\n", m.Name)
	}
}

func mapChoice(line string) string {
	switch line {
	case "1", "g", "grok", "xai":
		return "grok"
	case "2", "gemini", "gem":
		return "gemini"
	case "3", "claude", "anthropic":
		return "claude"
	case "4", "openai", "oai":
		return "openai"
	case "5", "ollama", "local":
		return "ollama"
	default:
		return ""
	}
}

func mustSource(name string) string {
	src, _ := KeySource(name)
	return src
}

func printDone(out io.Writer, active string) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "You're set.")
	if active != "" {
		fmt.Fprintf(out, "  Active provider: %s\n", active)
	}
	fmt.Fprintln(out, "  Type a question or /act <task>")
	fmt.Fprintln(out, "  Shift+Tab = BUILD → DESIGN → YOLO")
	fmt.Fprintln(out, "  /provider     — list keys + why active")
	fmt.Fprintln(out, "  /provider X   — switch without losing other keys")
	fmt.Fprintln(out, "  /setup        — add another key or change default")
	fmt.Fprintln(out)
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
