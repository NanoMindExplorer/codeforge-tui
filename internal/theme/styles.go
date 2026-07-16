package theme

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

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
	return lipgloss.NewStyle().Foreground(t.AccentAssistant)
}

func StyleAgent() lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().Foreground(t.AccentTool)
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
	return lipgloss.NewStyle().Bold(true).Foreground(t.AccentUser)
}

// AccentBar paints a vertical accent column (Grok scrollback block style).
func AccentBar(color lipgloss.Color, height int) string {
	if height < 1 {
		height = 1
	}
	line := lipgloss.NewStyle().Foreground(color).Render("┃")
	var b strings.Builder
	for i := 0; i < height; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

// BlockPrefix returns "┃ " in the role accent color.
func BlockPrefix(color lipgloss.Color) string {
	return lipgloss.NewStyle().Foreground(color).Render("┃ ")
}

// PaneBorder for optional side drawers.
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

// GradientBorder breathing top border (BorderActive → BorderGlow).
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

func StatusBarStyle(width int) lipgloss.Style {
	t := Current()
	return lipgloss.NewStyle().
		Background(t.BgSurface).
		Foreground(t.TextSecondary).
		Width(width)
}

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

// ModeBadge Grok-style session mode pills.
func ModeBadge(mode string) string {
	t := Current()
	var bg lipgloss.Color
	switch strings.ToUpper(mode) {
	case "PROMPT", "INSERT":
		bg = t.AccentUser
	case "SCROLL", "NORMAL":
		bg = t.Success
	case "COMMAND", "PALETTE":
		bg = t.Info
	case "PLAN":
		bg = t.AccentPlan
	case "ACT":
		bg = t.AccentRunning
	case "REVIEW":
		bg = t.AccentFocus
	default:
		bg = t.TextMuted
	}
	return lipgloss.NewStyle().
		Bold(true).
		Background(bg).
		Foreground(t.BgBase).
		Padding(0, 1).
		Render(mode)
}

// PromptFrame styles the bottom composer (Grok prompt widget).
func PromptFrame(width int, focused bool) lipgloss.Style {
	t := Current()
	border := t.PromptBorder
	if focused {
		border = t.AccentUser
	}
	pad := 1
	if CompactMode() {
		pad = 0
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(t.BgElevated).
		Padding(pad, 1).
		Width(width)
}

// Sparkline mini bars.
func Sparkline(values []float64) string {
	blocks := []rune{'▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	if len(values) == 0 {
		return strings.Repeat(string(blocks[0]), 8)
	}
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
	for lipgloss.Width(b.String()) < 8 {
		b.WriteRune(blocks[0])
	}
	return lipgloss.NewStyle().Foreground(t.AccentUser).Render(b.String())
}
