// Package skills discovers Grok-compatible SKILL.md packages and injects
// them into agent system prompts / slash commands (Phase G5).
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Source labels where a skill was found.
type Source string

const (
	SourceLocal  Source = "local"  // ./.codeforge/skills or ./.grok/skills
	SourceRepo   Source = "repo"   // same as local when at repo root
	SourceUser   Source = "user"   // ~/.codeforge or ~/.grok
	SourceClaude Source = "claude" // .claude/skills or ~/.claude/skills
	SourceCursor Source = "cursor" // .cursor/skills
	SourceExtra  Source = "extra"  // config paths
)

// Skill is one loaded SKILL.md package.
type Skill struct {
	Name        string
	Description string
	WhenToUse   string
	Body        string
	Path        string // absolute path to SKILL.md
	Dir         string // skill directory
	Source      Source
	// UserInvocable: available as /name slash command (default true)
	UserInvocable bool
	// DisableModelInvocation: hide from auto catalog in system prompt
	DisableModelInvocation bool
	Disabled               bool // config disabled list
}

// Registry holds discovered skills (deduped by name, higher priority wins).
type Registry struct {
	mu     sync.RWMutex
	order  []string
	byName map[string]*Skill
	// Disabled names from config
	disabled map[string]bool
}

var (
	globalMu sync.RWMutex
	global   *Registry
)

// Global returns the last loaded registry (never nil).
func Global() *Registry {
	globalMu.RLock()
	r := global
	globalMu.RUnlock()
	if r != nil {
		return r
	}
	return &Registry{byName: map[string]*Skill{}, disabled: map[string]bool{}}
}

// SetGlobal installs the process registry.
func SetGlobal(r *Registry) {
	globalMu.Lock()
	global = r
	globalMu.Unlock()
}

// Options for discovery.
type Options struct {
	WorkDir  string
	// ExtraPaths additional skill roots from config
	ExtraPaths []string
	// Ignore path prefixes to skip
	Ignore []string
	// Disabled skill names (still listed, not injected)
	Disabled []string
	// CompatClaude scan .claude/skills
	CompatClaude bool
	// CompatCursor scan .cursor/skills
	CompatCursor bool
}

// Load discovers skills for workdir and installs Global.
func Load(opt Options) *Registry {
	r := &Registry{
		byName:   map[string]*Skill{},
		disabled: map[string]bool{},
	}
	for _, d := range opt.Disabled {
		r.disabled[normalizeName(d)] = true
	}

	workdir := opt.WorkDir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(workdir); err == nil {
		workdir = abs
	}

	// Priority: local/repo first (later lower priority won't override)
	// We register high priority first; Register skips if name exists.
	type root struct {
		path string
		src  Source
	}
	var roots []root

	// Project-local (highest)
	for _, rel := range []string{
		".codeforge/skills",
		".grok/skills",
		".agents/skills",
	} {
		roots = append(roots, root{filepath.Join(workdir, rel), SourceLocal})
	}
	if opt.CompatClaude {
		roots = append(roots, root{filepath.Join(workdir, ".claude/skills"), SourceClaude})
	}
	if opt.CompatCursor {
		roots = append(roots, root{filepath.Join(workdir, ".cursor/skills"), SourceCursor})
	}

	// User home
	home, _ := os.UserHomeDir()
	if home != "" {
		for _, rel := range []string{
			".codeforge/skills",
			".grok/skills",
			".grok/bundled/skills",
		} {
			roots = append(roots, root{filepath.Join(home, rel), SourceUser})
		}
		if opt.CompatClaude {
			roots = append(roots, root{filepath.Join(home, ".claude/skills"), SourceClaude})
		}
		if opt.CompatCursor {
			roots = append(roots, root{filepath.Join(home, ".cursor/skills"), SourceCursor})
		}
	}

	// Extra config paths
	for _, p := range opt.ExtraPaths {
		p = expandHome(p)
		if p == "" {
			continue
		}
		roots = append(roots, root{p, SourceExtra})
	}

	for _, rt := range roots {
		if rt.path == "" {
			continue
		}
		if ignored(rt.path, opt.Ignore) {
			continue
		}
		r.scanRoot(rt.path, rt.src, opt.Ignore)
	}

	// Also flat commands/*.md as invocable skills (Claude legacy)
	for _, rel := range []string{".codeforge/commands", ".grok/commands", ".claude/commands"} {
		r.scanCommands(filepath.Join(workdir, rel), SourceLocal)
	}
	if home != "" {
		r.scanCommands(filepath.Join(home, ".codeforge/commands"), SourceUser)
		r.scanCommands(filepath.Join(home, ".grok/commands"), SourceUser)
	}

	SetGlobal(r)
	return r
}

func (r *Registry) scanRoot(root string, src Source, ignore []string) {
	info, err := os.Stat(root)
	if err != nil {
		return
	}
	// root can be a single SKILL.md or a directory of skill dirs
	if !info.IsDir() {
		if strings.EqualFold(filepath.Base(root), "SKILL.md") || strings.HasSuffix(strings.ToLower(root), ".md") {
			if sk, err := ParseFile(root, src); err == nil {
				r.register(sk)
			}
		}
		return
	}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ignored(path, ignore) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			// skip heavy dirs
			base := d.Name()
			if base == "node_modules" || base == ".git" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(d.Name(), "SKILL.md") {
			return nil
		}
		sk, err := ParseFile(path, src)
		if err != nil {
			return nil
		}
		r.register(sk)
		return nil
	})
}

func (r *Registry) scanCommands(dir string, src Source) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		if strings.EqualFold(e.Name(), "SKILL.md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		name = normalizeName(name)
		body := string(data)
		desc := firstParagraph(body)
		sk := &Skill{
			Name:          name,
			Description:   desc,
			Body:          stripFrontmatter(body),
			Path:          path,
			Dir:           dir,
			Source:        src,
			UserInvocable: true,
		}
		// try frontmatter if present
		if fm, rest, ok := splitFrontmatter(body); ok {
			applyFrontmatter(sk, fm)
			sk.Body = strings.TrimSpace(rest)
			if sk.Name == "" {
				sk.Name = name
			}
		}
		r.register(sk)
	}
}

func (r *Registry) register(sk *Skill) {
	if sk == nil || sk.Name == "" {
		return
	}
	sk.Name = normalizeName(sk.Name)
	if sk.Disabled || r.disabled[sk.Name] {
		sk.Disabled = true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byName[sk.Name]; exists {
		return // higher priority already registered
	}
	r.byName[sk.Name] = sk
	r.order = append(r.order, sk.Name)
}

// Get returns a skill by name.
func (r *Registry) Get(name string) (*Skill, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sk, ok := r.byName[normalizeName(name)]
	return sk, ok
}

// List returns skills in discovery order.
func (r *Registry) List() []*Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Skill, 0, len(r.order))
	for _, n := range r.order {
		if sk, ok := r.byName[n]; ok {
			out = append(out, sk)
		}
	}
	return out
}

// Count active (non-disabled) skills.
func (r *Registry) Count() int {
	n := 0
	for _, sk := range r.List() {
		if !sk.Disabled {
			n++
		}
	}
	return n
}

// Summary one-line for bootstrap logs.
func (r *Registry) Summary() string {
	all := r.List()
	active := r.Count()
	return fmt.Sprintf("skills: %d active (%d total)", active, len(all))
}

// CatalogForPrompt lists model-invocable skills (name + description) for the system prompt.
func (r *Registry) CatalogForPrompt() string {
	var lines []string
	for _, sk := range r.List() {
		if sk.Disabled || sk.DisableModelInvocation {
			continue
		}
		desc := sk.Description
		if desc == "" {
			desc = sk.WhenToUse
		}
		if desc == "" {
			desc = "(no description)"
		}
		if len(desc) > 160 {
			desc = desc[:157] + "…"
		}
		lines = append(lines, fmt.Sprintf("- /%s — %s", sk.Name, desc))
	}
	if len(lines) == 0 {
		return ""
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// InjectCatalog appends the skills catalog to a system prompt so the model
// knows which skills exist and can follow them when relevant.
func (r *Registry) InjectCatalog(system string) string {
	cat := r.CatalogForPrompt()
	if cat == "" {
		return system
	}
	var b strings.Builder
	b.WriteString(system)
	b.WriteString("\n\n# Available skills (Grok-compatible)\n")
	b.WriteString("When a task matches a skill description, follow that skill's instructions.\n")
	b.WriteString("Users may also invoke skills with /skill-name. Full skill body is injected on slash invoke.\n\n")
	b.WriteString(cat)
	return b.String()
}

// InvokePrompt builds the user/system message content when a skill is run via slash.
func (sk *Skill) InvokePrompt(args string) string {
	var b strings.Builder
	b.WriteString("# Skill: ")
	b.WriteString(sk.Name)
	b.WriteString("\n\n")
	if sk.Description != "" {
		b.WriteString(sk.Description)
		b.WriteString("\n\n")
	}
	b.WriteString("Follow these skill instructions carefully:\n\n")
	b.WriteString(sk.Body)
	if strings.TrimSpace(args) != "" {
		b.WriteString("\n\n## User arguments\n\n")
		b.WriteString(strings.TrimSpace(args))
	}
	b.WriteString("\n\nProceed with the skill now.")
	return b.String()
}

// RenderList human listing for /skills.
func (r *Registry) RenderList() string {
	all := r.List()
	if len(all) == 0 {
		return "No skills found.\n\nCreate one: ~/.codeforge/skills/<name>/SKILL.md\nOr project: .codeforge/skills/<name>/SKILL.md\nSee docs/SKILLS.md"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Skills (%d):\n", len(all))
	for _, sk := range all {
		mark := "  "
		if sk.Disabled {
			mark = "⊘ "
		}
		desc := sk.Description
		if len(desc) > 70 {
			desc = desc[:67] + "…"
		}
		inv := ""
		if sk.UserInvocable && !sk.Disabled {
			inv = "  → /" + sk.Name
		}
		fmt.Fprintf(&b, "%s%s [%s]%s\n    %s\n", mark, sk.Name, sk.Source, inv, desc)
	}
	b.WriteString("\n/skills <name>  — show full skill\n/skills reload  — rescan disk")
	return b.String()
}

// --- parsing ---

var nameClean = regexp.MustCompile(`[^a-z0-9-]+`)

func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = nameClean.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// ParseFile loads a SKILL.md path.
func ParseFile(path string, src Source) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(string(data), path, src)
}

// Parse parses SKILL.md content.
func Parse(content, path string, src Source) (*Skill, error) {
	sk := &Skill{
		Path:          path,
		Dir:           filepath.Dir(path),
		Source:        src,
		UserInvocable: true,
	}
	// default name from parent dir
	sk.Name = normalizeName(filepath.Base(filepath.Dir(path)))
	if strings.EqualFold(filepath.Base(path), "SKILL.md") == false {
		sk.Name = normalizeName(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	}

	body := content
	if fm, rest, ok := splitFrontmatter(content); ok {
		applyFrontmatter(sk, fm)
		body = rest
	}
	sk.Body = strings.TrimSpace(body)
	if sk.Description == "" {
		sk.Description = firstParagraph(sk.Body)
	}
	if sk.Name == "" {
		return nil, fmt.Errorf("skill name empty: %s", path)
	}
	return sk, nil
}

func splitFrontmatter(s string) (map[string]string, string, bool) {
	s = strings.TrimPrefix(s, "\uFEFF")
	if !strings.HasPrefix(strings.TrimSpace(s), "---") {
		return nil, s, false
	}
	// find first ---
	rest := strings.TrimSpace(s)
	rest = strings.TrimPrefix(rest, "---")
	rest = strings.TrimPrefix(rest, "\n")
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return nil, s, false
	}
	fmBlock := rest[:idx]
	body := strings.TrimSpace(rest[idx+4:]) // after \n---
	// strip optional newline after closing ---
	body = strings.TrimPrefix(body, "\n")
	fm := parseSimpleYAML(fmBlock)
	return fm, body, true
}

func stripFrontmatter(s string) string {
	if _, rest, ok := splitFrontmatter(s); ok {
		return rest
	}
	return s
}

// parseSimpleYAML handles flat key: value and simple lists for skill frontmatter.
func parseSimpleYAML(block string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(block, "\n")
	var listKey string
	var listVals []string
	flushList := func() {
		if listKey != "" {
			out[listKey] = strings.Join(listVals, ", ")
			listKey = ""
			listVals = nil
		}
	}
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		// list item
		if strings.HasPrefix(trim, "- ") && listKey != "" {
			listVals = append(listVals, strings.TrimSpace(trim[2:]))
			continue
		}
		flushList()
		if i := strings.Index(line, ":"); i > 0 {
			key := strings.TrimSpace(line[:i])
			val := strings.TrimSpace(line[i+1:])
			key = strings.ToLower(key)
			// unquote
			val = strings.Trim(val, `"'`)
			if val == "" {
				// maybe upcoming list
				listKey = key
				continue
			}
			// inline list [a, b]
			if strings.HasPrefix(val, "[") && strings.HasSuffix(val, "]") {
				val = strings.Trim(val, "[]")
			}
			out[key] = val
		}
	}
	flushList()
	return out
}

func applyFrontmatter(sk *Skill, fm map[string]string) {
	if v := fm["name"]; v != "" {
		sk.Name = normalizeName(v)
	}
	if v := fm["description"]; v != "" {
		sk.Description = v
	}
	if v := fm["when-to-use"]; v != "" {
		sk.WhenToUse = v
	} else if v := fm["when_to_use"]; v != "" {
		sk.WhenToUse = v
	}
	if v, ok := fm["user-invocable"]; ok {
		sk.UserInvocable = parseBool(v, true)
	} else if v, ok := fm["user_invocable"]; ok {
		sk.UserInvocable = parseBool(v, true)
	}
	if v, ok := fm["disable-model-invocation"]; ok {
		sk.DisableModelInvocation = parseBool(v, false)
	} else if v, ok := fm["disable_model_invocation"]; ok {
		sk.DisableModelInvocation = parseBool(v, false)
	}
}

func parseBool(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "1", "on":
		return true
	case "false", "no", "0", "off":
		return false
	default:
		return def
	}
}

func firstParagraph(s string) string {
	s = strings.TrimSpace(s)
	// skip headings
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 200 {
			return line[:197] + "…"
		}
		return line
	}
	return ""
}

func expandHome(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

func ignored(path string, ignore []string) bool {
	path = filepath.Clean(path)
	for _, ig := range ignore {
		ig = expandHome(ig)
		if ig == "" {
			continue
		}
		ig = filepath.Clean(ig)
		if path == ig || strings.HasPrefix(path, ig+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
