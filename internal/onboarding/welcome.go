package onboarding

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codeforge/tui/internal/config"
)

// StatusCard is the single first-run / welcome block (Q5.1).
// Designed for ~80 columns: brand + one status panel, no message flood.
func StatusCard(cfg *config.Config, activeName, activeModel string, healthy bool) string {
	var b strings.Builder
	// Compact ASCII (5 lines) + byline
	b.WriteString(BrandASCII())
	b.WriteString("\n  ")
	b.WriteString(BrandByline)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 48))
	b.WriteByte('\n')

	present := PresentCloudKeys()
	res := ResolveActive(cfg)

	if !healthy {
		b.WriteString("Status  ⚠  No API key yet\n")
		b.WriteString("  /setup gemini <AIza…>   free tier\n")
		b.WriteString("  /setup grok xai-…       Grok 4.5\n")
		b.WriteString("  export XAI_API_KEY=…    then restart\n")
	} else {
		b.WriteString(fmt.Sprintf("Status  ✓  %s", activeName))
		if activeModel != "" {
			b.WriteString(" · " + activeModel)
		}
		b.WriteByte('\n')
		if res.Reason != "" {
			b.WriteString("  why  " + res.Reason + "\n")
		}
		if src, _ := KeySource(activeName); src != "" {
			b.WriteString("  key  " + src + "\n")
		}
		if len(present) > 1 {
			var names []string
			for _, p := range present {
				if p.Name != normalizeName(activeName) {
					names = append(names, p.Name)
				}
			}
			if len(names) > 0 {
				b.WriteString(fmt.Sprintf("  also %s  → /provider\n", strings.Join(names, ", ")))
			}
		}
	}
	b.WriteString(strings.Repeat("─", 48))
	b.WriteString("\nShift+Tab modes · /help · /setup · /doctor\n")
	return b.String()
}

// WelcomeMessage is the TUI first-run system message (Q5.1: single status card).
// Alias of StatusCard for backward compatibility.
func WelcomeMessage(cfg *config.Config, activeName, activeModel string, healthy bool) string {
	return StatusCard(cfg, activeName, activeModel, healthy)
}

// EmptyStateNoKey is shown when chat is empty and no provider validates (Q5.2).
func EmptyStateNoKey() string {
	return `Nothing to send yet — add a key first:

  /setup gemini <AIza…>     free Google AI Studio key
  /setup grok xai-…         xAI Grok
  export GEMINI_API_KEY=…   then restart CodeForge

Or /setup for the full multi-provider guide.`
}

// EmptyStateNoProject is shown when workdir looks empty of code (Q5.2).
func EmptyStateNoProject(workdir string) string {
	base := filepath.Base(workdir)
	if base == "" || base == "." {
		base = "this folder"
	}
	return fmt.Sprintf(`Project "%s" has few or no source files yet.

  Tips:
  · Open CodeForge from a repo root (cd your-project && codeforge)
  · Or attach a file with @ and ask about it
  · /act scaffold a hello world   to start coding`, base)
}

// ProjectLooksEmpty reports whether workdir has almost no code files (Q5.2).
func ProjectLooksEmpty(workdir string) bool {
	if workdir == "" {
		return true
	}
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return true
	}
	codeExt := map[string]bool{
		".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
		".py": true, ".rs": true, ".java": true, ".c": true, ".h": true,
		".cpp": true, ".rb": true, ".php": true, ".swift": true, ".kt": true,
		".md": true, ".toml": true, ".yaml": true, ".yml": true, ".json": true,
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			// common project dirs count as non-empty
			switch name {
			case "src", "cmd", "pkg", "lib", "app", "internal", "tests", "test":
				return false
			}
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if codeExt[ext] {
			n++
			if n >= 2 {
				return false
			}
		}
	}
	return n < 2
}
