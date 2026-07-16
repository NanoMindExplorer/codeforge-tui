//go:build plainmd

package markdown

import "github.com/muesli/reflow/wordwrap"

// Build with -tags plainmd to exclude glamour/chroma from the binary.
func renderGlamour(src string, width int) string {
	return wordwrap.String(src, width)
}

// InvalidateRenderer is a no-op without glamour.
func InvalidateRenderer() {}
