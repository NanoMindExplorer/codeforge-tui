package theme

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

// ColorLevel matches terminal color capability.
type ColorLevel int

const (
	ColorNone ColorLevel = iota
	Color16
	Color256
	ColorTrue
)

var (
	levelOnce sync.Once
	cachedLvl ColorLevel
)

// DetectColorLevel inspects COLORTERM / TERM / NO_COLOR (cached).
func DetectColorLevel() ColorLevel {
	levelOnce.Do(func() {
		cachedLvl = detectColorLevel()
	})
	return cachedLvl
}

// ResetColorLevelCache clears the cache (tests).
func ResetColorLevelCache() {
	levelOnce = sync.Once{}
}

func detectColorLevel() ColorLevel {
	// Force overrides first (tests / CI / user preference)
	if v := os.Getenv("CODEFORGE_COLOR"); v != "" {
		switch strings.ToLower(v) {
		case "none", "0", "mono":
			return ColorNone
		case "16", "ansi":
			return Color16
		case "256":
			return Color256
		case "true", "truecolor", "24bit":
			return ColorTrue
		}
	}
	if os.Getenv("NO_COLOR") != "" {
		return ColorNone
	}
	ct := strings.ToLower(os.Getenv("COLORTERM"))
	if ct == "truecolor" || ct == "24bit" {
		return ColorTrue
	}
	// termenv profile
	switch termenv.ColorProfile() {
	case termenv.TrueColor:
		return ColorTrue
	case termenv.ANSI256:
		return Color256
	case termenv.ANSI:
		return Color16
	case termenv.Ascii:
		return ColorNone
	}
	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "truecolor") || strings.Contains(term, "24bit") {
		return ColorTrue
	}
	if strings.Contains(term, "256") {
		return Color256
	}
	if term == "" || term == "dumb" {
		return ColorNone
	}
	return Color16
}

// ColorLevelName human label.
func ColorLevelName(l ColorLevel) string {
	switch l {
	case ColorTrue:
		return "truecolor"
	case Color256:
		return "256"
	case Color16:
		return "16"
	default:
		return "none"
	}
}

// QuantizeColor maps an RGB hex (or ANSI index) to the terminal capability.
func QuantizeColor(c lipgloss.Color) lipgloss.Color {
	level := DetectColorLevel()
	return quantizeTo(c, level)
}

func quantizeTo(c lipgloss.Color, level ColorLevel) lipgloss.Color {
	s := string(c)
	if s == "" {
		return c
	}
	// Already an ANSI index (0-255 numeric, no #)
	if !strings.HasPrefix(s, "#") {
		if level == ColorNone {
			return lipgloss.Color("")
		}
		return c
	}
	switch level {
	case ColorTrue:
		return c
	case ColorNone:
		return lipgloss.Color("")
	case Color16:
		return lipgloss.Color(strconv.Itoa(nearestANSI16(s)))
	case Color256:
		return lipgloss.Color(strconv.Itoa(nearestXterm256(s)))
	default:
		return c
	}
}

// QuantizeTokens returns a copy of t with all colors quantized.
func QuantizeTokens(t Tokens) Tokens {
	level := DetectColorLevel()
	if level == ColorTrue {
		return t
	}
	q := func(c lipgloss.Color) lipgloss.Color { return quantizeTo(c, level) }
	t.BgBase = q(t.BgBase)
	t.BgSurface = q(t.BgSurface)
	t.BgElevated = q(t.BgElevated)
	t.BgOverlay = q(t.BgOverlay)
	t.BgLight = q(t.BgLight)
	t.BorderDim = q(t.BorderDim)
	t.BorderActive = q(t.BorderActive)
	t.BorderGlow = q(t.BorderGlow)
	t.PromptBorder = q(t.PromptBorder)
	t.SelectionBorder = q(t.SelectionBorder)
	t.TextPrimary = q(t.TextPrimary)
	t.TextSecondary = q(t.TextSecondary)
	t.TextMuted = q(t.TextMuted)
	t.TextDisabled = q(t.TextDisabled)
	t.AccentUser = q(t.AccentUser)
	t.AccentAssistant = q(t.AccentAssistant)
	t.AccentTool = q(t.AccentTool)
	t.AccentThinking = q(t.AccentThinking)
	t.AccentSystem = q(t.AccentSystem)
	t.AccentPlan = q(t.AccentPlan)
	t.AccentRunning = q(t.AccentRunning)
	t.AccentAI = q(t.AccentAI)
	t.AccentAgent = q(t.AccentAgent)
	t.AccentFocus = q(t.AccentFocus)
	t.Success = q(t.Success)
	t.Danger = q(t.Danger)
	t.Warning = q(t.Warning)
	t.Info = q(t.Info)
	t.DiffAddBg = q(t.DiffAddBg)
	t.DiffAddFg = q(t.DiffAddFg)
	t.DiffDelBg = q(t.DiffDelBg)
	t.DiffDelFg = q(t.DiffDelFg)
	t.DiffCtxFg = q(t.DiffCtxFg)
	t.ScrollbarBg = q(t.ScrollbarBg)
	t.ScrollbarFg = q(t.ScrollbarFg)
	t.MdHeading = q(t.MdHeading)
	t.MdLink = q(t.MdLink)
	t.MdCode = q(t.MdCode)
	t.MdCodeBg = q(t.MdCodeBg)
	return t
}

// ansi16 palette (standard)
var ansi16Hex = []string{
	"#000000", "#800000", "#008000", "#808000",
	"#000080", "#800080", "#008080", "#c0c0c0",
	"#808080", "#ff0000", "#00ff00", "#ffff00",
	"#0000ff", "#ff00ff", "#00ffff", "#ffffff",
}

func nearestANSI16(hex string) int {
	c, err := colorful.Hex(hex)
	if err != nil {
		return 7
	}
	best, bestD := 7, 1e9
	for i, h := range ansi16Hex {
		o, err := colorful.Hex(h)
		if err != nil {
			continue
		}
		d := c.DistanceLab(o)
		if d < bestD {
			bestD = d
			best = i
		}
	}
	return best
}

func nearestXterm256(hex string) int {
	c, err := colorful.Hex(hex)
	if err != nil {
		return 7
	}
	// Prefer 16 system colors when close enough (cleaner on dark themes)
	best, bestD := nearestANSI16(hex), 1e9
	for i, h := range ansi16Hex {
		o, _ := colorful.Hex(h)
		d := c.DistanceLab(o)
		if d < bestD {
			bestD = d
			best = i
		}
	}
	// xterm 6x6x6 cube (16–231)
	for r := 0; r < 6; r++ {
		for g := 0; g < 6; g++ {
			for b := 0; b < 6; b++ {
				idx := 16 + 36*r + 6*g + b
				rr := cubeLevel(r)
				gg := cubeLevel(g)
				bb := cubeLevel(b)
				o := colorful.Color{R: float64(rr) / 255, G: float64(gg) / 255, B: float64(bb) / 255}
				d := c.DistanceLab(o)
				if d < bestD {
					bestD = d
					best = idx
				}
			}
		}
	}
	// grayscale 232–255
	for i := 0; i < 24; i++ {
		v := 8 + i*10
		o := colorful.Color{R: float64(v) / 255, G: float64(v) / 255, B: float64(v) / 255}
		d := c.DistanceLab(o)
		if d < bestD {
			bestD = d
			best = 232 + i
		}
	}
	return best
}

func cubeLevel(i int) int {
	if i == 0 {
		return 0
	}
	return 55 + 40*i
}
