package components

import (
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// Toast is a short-lived status message.
type Toast struct {
	Message string
	Kind    string // success | error | info | warning
	Until   time.Time
}

// NewToast creates a toast that expires after d.
func NewToast(msg, kind string, d time.Duration) Toast {
	return Toast{Message: msg, Kind: kind, Until: time.Now().Add(d)}
}

// Alive reports if the toast should still show.
func (t Toast) Alive() bool {
	return t.Message != "" && time.Now().Before(t.Until)
}

// View renders the toast bar.
func (t Toast) View(width int) string {
	if !t.Alive() {
		return ""
	}
	tok := theme.Current()
	var fg lipgloss.Color
	icon := "ℹ"
	switch t.Kind {
	case "success":
		fg = tok.Success
		icon = "✓"
	case "error":
		fg = tok.Danger
		icon = "✗"
	case "warning":
		fg = tok.Warning
		icon = "⚠"
	default:
		fg = tok.Info
	}
	return lipgloss.NewStyle().
		Background(tok.BgOverlay).
		Foreground(fg).
		Width(width).
		Padding(0, 1).
		Render(icon + " " + t.Message)
}
