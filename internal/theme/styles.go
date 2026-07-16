package theme

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

// Style helpers — all styling flows through Current() tokens.

func StyleTextPrimary() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.TextPrimary)
}

func StyleTextSecondary() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.TextSecondary)
}

func StyleTextMuted() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.TextMuted).Italic(true)
}

func StyleUser() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.AccentUser).Bold(true)
}

func StyleAI() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.AccentAI)
}

func StyleAgent() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.AccentAgent)
}

func StyleSuccess() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.Success)
}

func StyleDanger() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.Danger)
}

func StyleWarning() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.Warning)
}

func StyleHeader() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Bold(true).Foreground(t.AccentAI)
}

// PaneBorder returns a rounded border style for a pane.
// When active and motion is on, border uses BorderActive; otherwise BorderDim.
func PaneBorder(active bool, width, height int) lipgloss.Style {
	t := Current()
	borderColor := t.BorderDim
	bg := t.BgSurface
	if active {
		borderColor = t.BorderActive
		bg = t.BgElevated
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Background(bg).
		Padding(0, 1).
		Width(width).
		Height(height)
}

// GradientBorder draws a breathing gradient top border (BorderActive → BorderGlow).
// t is the animation phase [0,1). Returns a string of colored ─ characters.
func GradientBorder(width int, phase float64) string {
	if width <= 0 {
		return ""
	}
	tok := Current()
	from, err1 := colorful.Hex(string(tok.BorderActive))
	to, err2 := colorful.Hex(string(tok.BorderGlow))
	if err1 != nil || err2 != nil {
		return lipgloss.NewStyle().Foreground(tok.BorderActive).Render(strings.Repeat("─", width))
	}
	if !MotionEnabled() {
		return lipgloss.NewStyle().Foreground(tok.BorderActive).Render(strings.Repeat("─", width))
	}
	var b strings.Builder
	for i := 0; i < width; i++ {
		pos := math.Mod(float64(i)/float64(width)+phase, 1.0)
		c := from.BlendLuv(to, pos)
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex())).Render("─"))
	}
	return b.String()
}

// StatusBarBg is the top/bottom bar background style.
func StatusBarStyle(width int) lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().
		Background(t.BgElevated).
		Foreground(t.TextSecondary).
		Width(width)
}

// OverlayStyle for modals / command palette.
func OverlayStyle(width int) lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().
		Background(t.BgOverlay).
		Foreground(t.TextPrimary).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderGlow).
		Padding(1, 2).
		Width(width)
}

// ModeBadge returns a colored badge for NORMAL/INSERT/COMMAND/PLAN/ACT.
func ModeBadge(mode string) string {
	t := Current()
	var bg lipgloss.Color
	switch mode {
	case "NORMAL":
		bg = t.Success
	case "INSERT":
		bg = t.Warning
	case "COMMAND":
		bg = t.Info
	case "PLAN":
		bg = t.AccentAI
	case "ACT":
		bg = t.AccentAgent
	case "REVIEW":
		bg = t.AccentFocus
	default:
		bg = t.TextMuted
	}
	return lipgloss.NewStyle().
		Bold(true).
		Background(bg).
		Foreground(lipgloss.Color("#0A0E14")).
		Padding(0, 1).
		Render(mode)
}

// Sparkline renders a mini bar chart from values (0..1 each), max 8 chars.
func Sparkline(values []float64) string {
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	if len(values) == 0 {
		return strings.Repeat(string(blocks[0]), 8)
	}
	// Take last 8
	start := 0
	if len(values) > 8 {
		start = len(values) - 8
	}
	t := Current()
	var b strings.Builder
	for _, v := range values[start:] {
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		idx := int(v * float64(len(blocks)-1))
		b.WriteRune(blocks[idx])
	}
	for b.Len() < 8 {
		b.WriteRune(blocks[0])
	}
	return lipgloss.NewStyle().Foreground(t.AccentAI).Render(b.String())
}
