// Package personas loads Grok-compatible behavioral overlays for subagents (Phase G6).
package personas

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Persona is a named behavioral overlay for subagents.
type Persona struct {
	Name         string `yaml:"name" json:"name"`
	Description  string `yaml:"description" json:"description"`
	Instructions string `yaml:"instructions" json:"instructions"`
	// InstructionsFile loaded and appended after Instructions.
	InstructionsFile string `yaml:"instructions_file" json:"instructions_file"`
	// Model optional model hint (not enforced unless runner supports it).
	Model string `yaml:"model" json:"model"`
	// DefaultIsolation: none | worktree
	DefaultIsolation string `yaml:"default_isolation" json:"default_isolation"`
	// Source path or "config" / "bundled"
	Source string `yaml:"-" json:"source,omitempty"`
	// Resolved full instructions (after file merge)
	Resolved string `yaml:"-" json:"-"`
}

// Registry of personas by name.
type Registry struct {
	mu     sync.RWMutex
	order  []string
	byName map[string]*Persona
}

var (
	globalMu sync.RWMutex
	global   *Registry
)

// Global returns last loaded registry (never nil).
func Global() *Registry {
	globalMu.RLock()
	r := global
	globalMu.RUnlock()
	if r != nil {
		return r
	}
	return &Registry{byName: map[string]*Persona{}}
}

// SetGlobal installs process registry.
func SetGlobal(r *Registry) {
	globalMu.Lock()
	global = r
	globalMu.Unlock()
}

// Options for discovery.
type Options struct {
	WorkDir string
	// ConfigPersonas from YAML config map name → persona
	ConfigPersonas map[string]Persona
	// ExtraDirs additional persona directories
	ExtraDirs []string
}

// Load discovers personas and sets Global.
func Load(opt Options) *Registry {
	r := &Registry{byName: map[string]*Persona{}}
	workdir := opt.WorkDir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}

	// Bundled defaults first (lowest priority — register first, higher overrides)
	for _, p := range bundled() {
		r.register(p)
	}

	home, _ := os.UserHomeDir()
	dirs := []string{}
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".codeforge", "personas"),
			filepath.Join(home, ".grok", "personas"),
		)
	}
	dirs = append(dirs,
		filepath.Join(workdir, ".codeforge", "personas"),
		filepath.Join(workdir, ".grok", "personas"),
	)
	dirs = append(dirs, opt.ExtraDirs...)

	for _, d := range dirs {
		r.scanDir(d)
	}

	// Config overrides everything
	for name, p := range opt.ConfigPersonas {
		cp := p
		if cp.Name == "" {
			cp.Name = name
		}
		cp.Source = "config"
		r.register(&cp)
	}

	// Resolve instruction files
	for _, p := range r.List() {
		p.Resolved = resolveInstructions(p, workdir)
	}

	SetGlobal(r)
	return r
}

func (r *Registry) scanDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		low := strings.ToLower(name)
		if !strings.HasSuffix(low, ".yaml") && !strings.HasSuffix(low, ".yml") && !strings.HasSuffix(low, ".toml") {
			continue
		}
		path := filepath.Join(dir, name)
		p, err := ParseFile(path)
		if err != nil {
			continue
		}
		if p.Name == "" {
			p.Name = strings.TrimSuffix(name, filepath.Ext(name))
		}
		p.Source = path
		r.register(p)
	}
}

func (r *Registry) register(p *Persona) {
	if p == nil {
		return
	}
	name := normalize(p.Name)
	if name == "" {
		return
	}
	p.Name = name
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[name]; exists {
		// replace (higher priority)
		r.byName[name] = p
		return
	}
	r.byName[name] = p
	r.order = append(r.order, name)
}

// Get returns a persona by name.
func (r *Registry) Get(name string) (*Persona, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.byName[normalize(name)]
	return p, ok
}

// List in registration order.
func (r *Registry) List() []*Persona {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Persona, 0, len(r.order))
	for _, n := range r.order {
		if p, ok := r.byName[n]; ok {
			out = append(out, p)
		}
	}
	// also any replaced-only keys
	for n, p := range r.byName {
		found := false
		for _, o := range r.order {
			if o == n {
				found = true
				break
			}
		}
		if !found {
			out = append(out, p)
		}
	}
	return out
}

func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byName)
}

func (r *Registry) Summary() string {
	return fmt.Sprintf("personas: %d", r.Count())
}

// RenderList for /personas slash.
func (r *Registry) RenderList() string {
	all := r.List()
	if len(all) == 0 {
		return "No personas.\n\nAdd ~/.codeforge/personas/<name>.yaml\nSee docs/SUBAGENTS.md"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Personas (%d):\n", len(all))
	for _, p := range all {
		desc := p.Description
		if desc == "" {
			desc = firstLine(p.Resolved)
		}
		if len(desc) > 72 {
			desc = desc[:69] + "…"
		}
		src := p.Source
		if len(src) > 40 {
			src = "…" + src[len(src)-37:]
		}
		fmt.Fprintf(&b, "  %s\n    %s\n    (%s)\n", p.Name, desc, src)
	}
	b.WriteString("\nSpawn: spawn_subagent persona=<name>\n/personas <name> — show instructions")
	return b.String()
}

// SystemReminder wraps persona instructions for injection into subagent system prompt.
func (p *Persona) SystemReminder() string {
	body := strings.TrimSpace(p.Resolved)
	if body == "" {
		body = strings.TrimSpace(p.Instructions)
	}
	if body == "" {
		return ""
	}
	return "<system-reminder>\nPersona: " + p.Name + "\n" + body + "\n</system-reminder>"
}

// ParseFile loads yaml or minimal toml.
func ParseFile(path string) (*Persona, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	low := strings.ToLower(path)
	if strings.HasSuffix(low, ".toml") {
		return parseTOML(string(data), path)
	}
	var p Persona
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

func parseTOML(s, path string) (*Persona, error) {
	// Minimal TOML: key = "value" or key = '''multiline'''
	p := &Persona{}
	lines := strings.Split(s, "\n")
	var curKey string
	var multi strings.Builder
	inMulti := false
	multiQuote := ""

	flushMulti := func() {
		if curKey == "" {
			return
		}
		val := strings.TrimSpace(multi.String())
		setPersonaField(p, curKey, val)
		curKey = ""
		multi.Reset()
		inMulti = false
	}

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if inMulti {
			if strings.Contains(line, multiQuote) {
				// end
				before, _, _ := strings.Cut(line, multiQuote)
				multi.WriteString(before)
				flushMulti()
				continue
			}
			multi.WriteString(line)
			multi.WriteByte('\n')
			continue
		}
		if trim == "" || strings.HasPrefix(trim, "#") || strings.HasPrefix(trim, "[") {
			continue
		}
		i := strings.Index(line, "=")
		if i < 0 {
			continue
		}
		key := strings.TrimSpace(line[:i])
		val := strings.TrimSpace(line[i+1:])
		if strings.HasPrefix(val, "'''") || strings.HasPrefix(val, `"""`) {
			multiQuote = val[:3]
			rest := val[3:]
			if strings.HasSuffix(rest, multiQuote) && len(rest) >= 3 {
				setPersonaField(p, key, strings.TrimSuffix(rest, multiQuote))
				continue
			}
			inMulti = true
			curKey = key
			multi.WriteString(rest)
			if rest != "" {
				multi.WriteByte('\n')
			}
			continue
		}
		val = strings.Trim(val, `"'`)
		setPersonaField(p, key, val)
	}
	if p.Name == "" {
		p.Name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	return p, nil
}

func setPersonaField(p *Persona, key, val string) {
	switch strings.ToLower(strings.ReplaceAll(key, "-", "_")) {
	case "name":
		p.Name = val
	case "description":
		p.Description = val
	case "instructions":
		p.Instructions = val
	case "instructions_file":
		p.InstructionsFile = val
	case "model":
		p.Model = val
	case "default_isolation":
		p.DefaultIsolation = val
	}
}

func resolveInstructions(p *Persona, workdir string) string {
	var parts []string
	if strings.TrimSpace(p.Instructions) != "" {
		parts = append(parts, strings.TrimSpace(p.Instructions))
	}
	if f := strings.TrimSpace(p.InstructionsFile); f != "" {
		path := f
		if !filepath.IsAbs(path) {
			// try relative to persona source dir, then workdir
			if p.Source != "" && p.Source != "config" && p.Source != "bundled" {
				cand := filepath.Join(filepath.Dir(p.Source), f)
				if _, err := os.Stat(cand); err == nil {
					path = cand
				} else {
					path = filepath.Join(workdir, f)
				}
			} else {
				path = filepath.Join(workdir, f)
			}
		}
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, strings.TrimSpace(string(data)))
		}
	}
	return strings.Join(parts, "\n\n")
}

func bundled() []*Persona {
	return []*Persona{
		{
			Name:        "researcher",
			Description: "Thorough investigator; cite file paths.",
			Instructions: `You are a thorough researcher.
- Always cite specific file paths and line ranges when possible.
- Prefer evidence over speculation.
- End with a short bullet summary of findings.`,
			Source: "bundled",
		},
		{
			Name:        "concise",
			Description: "Short, high-signal answers.",
			Instructions: `Be concise.
- Prefer bullets over paragraphs.
- No preamble or filler.
- Lead with the answer, then minimal detail.`,
			Source: "bundled",
		},
		{
			Name:        "reviewer",
			Description: "Code review focus: bugs, security, tests.",
			Instructions: `You are a careful code reviewer.
- Look for bugs, security issues, missing tests, and API breakage.
- Severity-tag findings: blocker / major / minor / nit.
- Suggest concrete fixes with file paths.`,
			Source: "bundled",
		},
	}
}

func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s
}
