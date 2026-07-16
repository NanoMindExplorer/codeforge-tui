// Package rules loads project instructions (AGENTS.md, CLAUDE.md, etc.)
// and injects them into agent / chat system prompts.
package rules

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Default candidate files, in priority order (first found wins for "primary",
// all found are merged).
var candidates = []string{
	"AGENTS.md",
	"CLAUDE.md",
	"CODEFORGE.md",
	".codeforge/rules.md",
	".codeforge/AGENTS.md",
	".cursorrules",
	".github/copilot-instructions.md",
}

// Bundle is the loaded project memory.
type Bundle struct {
	Primary string   // path of first hit
	Paths   []string // all loaded paths
	Text    string   // merged content
}

var (
	mu     sync.RWMutex
	cached *Bundle
)

// Load scans workdir (and optional extra roots) for rule files.
func Load(workdir string, extraRoots ...string) *Bundle {
	seen := map[string]bool{}
	var parts []string
	var paths []string
	primary := ""

	scan := func(root string) {
		for _, name := range candidates {
			p := filepath.Join(root, name)
			if seen[p] {
				continue
			}
			data, err := os.ReadFile(p)
			if err != nil || len(data) == 0 {
				continue
			}
			seen[p] = true
			if primary == "" {
				primary = p
			}
			paths = append(paths, p)
			// Cap each file
			txt := string(data)
			if len(txt) > 24_000 {
				txt = txt[:24_000] + "\n… (rules truncated)"
			}
			parts = append(parts, "## From "+filepath.Base(p)+" ("+p+")\n\n"+txt)
		}
	}

	scan(workdir)
	for _, r := range extraRoots {
		if r != "" {
			scan(r)
		}
	}

	b := &Bundle{Primary: primary, Paths: paths, Text: strings.Join(parts, "\n\n---\n\n")}
	mu.Lock()
	cached = b
	mu.Unlock()
	return b
}

// Get returns the last loaded bundle (may be empty).
func Get() *Bundle {
	mu.RLock()
	defer mu.RUnlock()
	if cached == nil {
		return &Bundle{}
	}
	return cached
}

// Inject appends rules to a system prompt.
func Inject(system string, b *Bundle) string {
	if b == nil || strings.TrimSpace(b.Text) == "" {
		return system
	}
	var sb strings.Builder
	sb.WriteString(system)
	sb.WriteString("\n\n# Project rules (must follow)\n")
	sb.WriteString("The following project-specific instructions override defaults when they conflict.\n\n")
	// Overall cap
	txt := b.Text
	if len(txt) > 32_000 {
		txt = txt[:32_000] + "\n… (truncated)"
	}
	sb.WriteString(txt)
	return sb.String()
}

// Summary is a short UI line.
func (b *Bundle) Summary() string {
	if b == nil || len(b.Paths) == 0 {
		return "No project rules found (add AGENTS.md)"
	}
	return "Rules: " + strings.Join(basenameAll(b.Paths), ", ")
}

func basenameAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}
