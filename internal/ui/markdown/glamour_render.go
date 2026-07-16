//go:build !plainmd

package markdown

import (
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/codeforge/tui/internal/theme"
	"github.com/muesli/reflow/wordwrap"
)

type glamRenderer = *glamour.TermRenderer

func renderGlamour(src string, width int) string {
	r, err := getRenderer(width)
	if err != nil || r == nil {
		return wordwrap.String(src, width)
	}
	out, err := r.Render(src)
	if err != nil {
		return wordwrap.String(src, width)
	}
	return strings.TrimRight(out, "\n")
}

func getRenderer(width int) (*glamour.TermRenderer, error) {
	mu.Lock()
	defer mu.Unlock()
	if disabled {
		return nil, nil
	}
	styleName := theme.GlamourStyleName()
	if renderer != nil && lastW == width && lastStyle == styleName {
		if gr, ok := renderer.(*glamour.TermRenderer); ok {
			return gr, nil
		}
	}
	std := styles.DarkStyle
	switch styleName {
	case "light":
		std = styles.LightStyle
	case "dracula":
		std = styles.DraculaStyle
	case "notty":
		std = styles.NoTTYStyle
	case "pink":
		std = styles.PinkStyle
	default:
		std = styles.DarkStyle
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle(std),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, err
	}
	renderer = r
	lastW = width
	lastStyle = styleName
	return r, nil
}

// InvalidateRenderer drops the cached glamour renderer (call after theme switch).
func InvalidateRenderer() {
	mu.Lock()
	renderer = nil
	lastW = 0
	lastStyle = ""
	mu.Unlock()
}
