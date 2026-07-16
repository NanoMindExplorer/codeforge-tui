// Package markdown renders AI responses with glamour + CodeForge theme.
// Rendering is lazy and can be disabled for smaller memory / faster start.
package markdown

import (
	"os"
	"strings"
	"sync"
	"unicode"

	"github.com/muesli/reflow/wordwrap"
)

var (
	mu        sync.Mutex
	renderer  any
	lastW     int
	lastStyle string
	disabled  bool
	initOnce  sync.Once
)

func initFlags() {
	initOnce.Do(func() {
		v := strings.ToLower(os.Getenv("CODEFORGE_PLAIN_MD"))
		if v == "1" || v == "true" || v == "yes" {
			disabled = true
		}
		if strings.ToLower(os.Getenv("CODEFORGE_NO_GLAMOUR")) == "1" {
			disabled = true
		}
	})
}

// SetPlain forces plain wrapping (no glamour) — useful for tests / low memory.
func SetPlain(on bool) {
	mu.Lock()
	disabled = on
	mu.Unlock()
}

// Render converts markdown to styled terminal output at the given width.
func Render(src string, width int) string {
	initFlags()
	if width < 20 {
		width = 20
	}
	if width > 120 {
		width = 120
	}
	// Fast path: no markdown markers → plain wrap (avoids glamour/chroma cost)
	if disabled || !looksLikeMarkdown(src) {
		return wordwrap.String(src, width)
	}
	return renderGlamour(src, width)
}

// WrapANSI wraps text without counting ANSI escape codes (via reflow).
func WrapANSI(s string, width int) string {
	if width <= 0 {
		width = 40
	}
	return wordwrap.String(s, width)
}

func looksLikeMarkdown(s string) bool {
	if len(s) < 2 {
		return false
	}
	// Heuristics: code fences, headings, lists, links, bold
	if strings.Contains(s, "```") || strings.Contains(s, "## ") || strings.Contains(s, "### ") {
		return true
	}
	if strings.Contains(s, "](") || strings.Contains(s, "**") || strings.Contains(s, "- [ ]") {
		return true
	}
	// lines starting with # or -
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimLeftFunc(line, unicode.IsSpace)
		if strings.HasPrefix(t, "# ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "> ") {
			return true
		}
	}
	return false
}
