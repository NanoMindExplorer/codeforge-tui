// Package markdown renders AI responses with glamour + CodeForge theme.
package markdown

import (
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/reflow/wordwrap"
)

var (
	mu       sync.Mutex
	renderer *glamour.TermRenderer
	lastW    int
)

// Render converts markdown to styled terminal output at the given width.
func Render(src string, width int) string {
	if width < 20 {
		width = 20
	}
	// Soft limit for performance
	if width > 120 {
		width = 120
	}
	r, err := getRenderer(width)
	if err != nil || r == nil {
		return wordwrap.String(src, width)
	}
	out, err := r.Render(src)
	if err != nil {
		return wordwrap.String(src, width)
	}
	// glamour adds trailing newlines
	return strings.TrimRight(out, "\n")
}

// WrapANSI wraps already-styled text without counting ANSI escape codes.
func WrapANSI(s string, width int) string {
	if width <= 0 {
		width = 40
	}
	return wordwrap.String(s, width)
}

func getRenderer(width int) (*glamour.TermRenderer, error) {
	mu.Lock()
	defer mu.Unlock()
	if renderer != nil && lastW == width {
		return renderer, nil
	}
	// Dark style matches Aurora aesthetic
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(styles.DarkStyle),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	renderer = r
	lastW = width
	return r, nil
}
