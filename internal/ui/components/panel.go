package components

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// Panel renders a titled bordered region.
func Panel(title, body string, width, height int, active bool, phase float64) string {
	t := theme.Current()
	innerW := width - 4
	if innerW < 4 {
		innerW = 4
	}

	// Title row with optional gradient underline when active
	titleStyled := theme.StyleHeader().Render(title)
	var top string
	if active && theme.MotionEnabled() {
		top = titleStyled + "\n" + theme.GradientBorder(innerW, phase)
	} else {
		col := t.BorderDim
		if active {
			col = t.BorderActive
		}
		top = titleStyled + "\n" + lipgloss.NewStyle().Foreground(col).Render(strings.Repeat("─", innerW))
	}

	content := top + "\n" + body
	return theme.PaneBorder(active, width, height).Render(content)
}

// Badge renders a small colored pill.
func Badge(text string, color lipgloss.Color) string {
	return lipgloss.NewStyle().
		Background(color).
		Foreground(lipgloss.Color("#0A0E14")).
		Bold(true).
		Padding(0, 1).
		Render(text)
}
