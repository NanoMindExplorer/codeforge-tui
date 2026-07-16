// Package theme is the single source of truth for CodeForge visual tokens.
// No lipgloss.Color("#...") literals should live outside this package.
package theme

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// Tokens holds the full design-system palette (Aurora Dark by default).
type Tokens struct {
	// Background elevation (4 layers)
	BgBase     lipgloss.Color
	BgSurface  lipgloss.Color
	BgElevated lipgloss.Color
	BgOverlay  lipgloss.Color

	// Borders
	BorderDim    lipgloss.Color
	BorderActive lipgloss.Color
	BorderGlow   lipgloss.Color

	// Text hierarchy
	TextPrimary   lipgloss.Color
	TextSecondary lipgloss.Color
	TextMuted     lipgloss.Color
	TextDisabled  lipgloss.Color

	// Role accents
	AccentAI    lipgloss.Color // cyan  — AI / system
	AccentAgent lipgloss.Color // violet — agent tool calls
	AccentUser  lipgloss.Color // sky   — user messages
	AccentFocus lipgloss.Color // pink  — cursor / selection

	// Semantic status
	Success lipgloss.Color
	Danger  lipgloss.Color
	Warning lipgloss.Color
	Info    lipgloss.Color

	// Diff
	DiffAddBg lipgloss.Color
	DiffAddFg lipgloss.Color
	DiffDelBg lipgloss.Color
	DiffDelFg lipgloss.Color
	DiffCtxFg lipgloss.Color
}

// Aurora returns the default "Aurora Dark" theme.
func Aurora() Tokens {
	return Tokens{
		BgBase: "#0A0E14", BgSurface: "#10151C", BgElevated: "#161C26", BgOverlay: "#1C2430",
		BorderDim: "#232B38", BorderActive: "#22D3EE", BorderGlow: "#A78BFA",
		TextPrimary: "#E6EDF3", TextSecondary: "#8B98A8", TextMuted: "#576273", TextDisabled: "#384250",
		AccentAI: "#22D3EE", AccentAgent: "#A78BFA", AccentUser: "#38BDF8", AccentFocus: "#F472B6",
		Success: "#34D399", Danger: "#FB7185", Warning: "#FBBF24", Info: "#60A5FA",
		DiffAddBg: "#0F2419", DiffAddFg: "#34D399", DiffDelBg: "#2A1215", DiffDelFg: "#FB7185", DiffCtxFg: "#576273",
	}
}

// Light returns a light-mode variant for AdaptiveColor use.
func Light() Tokens {
	return Tokens{
		BgBase: "#F8FAFC", BgSurface: "#F1F5F9", BgElevated: "#E2E8F0", BgOverlay: "#CBD5E1",
		BorderDim: "#CBD5E1", BorderActive: "#0891B2", BorderGlow: "#7C3AED",
		TextPrimary: "#0F172A", TextSecondary: "#475569", TextMuted: "#94A3B8", TextDisabled: "#CBD5E1",
		AccentAI: "#0891B2", AccentAgent: "#7C3AED", AccentUser: "#0284C7", AccentFocus: "#DB2777",
		Success: "#059669", Danger: "#E11D48", Warning: "#D97706", Info: "#2563EB",
		DiffAddBg: "#D1FAE5", DiffAddFg: "#047857", DiffDelBg: "#FEE2E2", DiffDelFg: "#BE123C", DiffCtxFg: "#94A3B8",
	}
}

var (
	mu      sync.RWMutex
	current = Aurora()
	motion  = true
)

// Current returns the active theme tokens (thread-safe copy).
func Current() Tokens {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set replaces the active theme.
func Set(t Tokens) {
	mu.Lock()
	current = t
	mu.Unlock()
}

// MotionEnabled reports whether animations should run.
func MotionEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return motion
}

// SetMotion enables/disables animations (--no-motion / CODEFORGE_NO_MOTION).
func SetMotion(on bool) {
	mu.Lock()
	motion = on
	mu.Unlock()
}

// InitFromEnv applies CODEFORGE_THEME and CODEFORGE_NO_MOTION.
func InitFromEnv() {
	if v := os.Getenv("CODEFORGE_NO_MOTION"); v == "1" || strings.EqualFold(v, "true") {
		SetMotion(false)
	}
	name := os.Getenv("CODEFORGE_THEME")
	if name == "" {
		name = "aurora"
	}
	switch strings.ToLower(name) {
	case "light", "adaptive":
		Set(Light())
	default:
		Set(Aurora())
	}
	// Optional YAML override
	if t, err := LoadFromFile(""); err == nil && t != nil {
		Set(*t)
	}
}

// HasNerdFont detects Nerd Font availability via env or sample glyph.
func HasNerdFont() bool {
	if v := os.Getenv("NERD_FONT"); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	if v := os.Getenv("NERD_FONTS"); v != "" {
		return true
	}
	// Heuristic: TERM_PROGRAM / common terminals that ship with Nerd Fonts
	tp := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	return tp == "iTerm.app" || tp == "WezTerm" || tp == "ghostty"
}

// FileIcon returns a glyph for a file extension (Nerd Font or safe Unicode).
func FileIcon(name string) string {
	ext := strings.ToLower(name)
	if i := strings.LastIndex(name, "."); i >= 0 {
		ext = strings.ToLower(name[i:])
	}
	if HasNerdFont() {
		switch ext {
		case ".go":
			return ""
		case ".md":
			return "󰍔"
		case ".json", ".yaml", ".yml", ".toml":
			return ""
		case ".git":
			return ""
		case ".py":
			return ""
		case ".js", ".ts", ".tsx", ".jsx":
			return ""
		case ".rs":
			return ""
		default:
			return "󰈔"
		}
	}
	switch ext {
	case ".go":
		return "◆"
	case ".md":
		return "▣"
	case ".json", ".yaml", ".yml", ".toml":
		return "◇"
	default:
		return "●"
	}
}

// GitStatusGlyph returns a short glyph for git status.
func GitStatusGlyph(status string) string {
	switch status {
	case "M", "modified":
		if HasNerdFont() {
			return ""
		}
		return "●"
	case "A", "new", "??":
		if HasNerdFont() {
			return ""
		}
		return "○"
	case "D", "deleted":
		if HasNerdFont() {
			return ""
		}
		return "✕"
	case "staged":
		if HasNerdFont() {
			return "󱁿"
		}
		return "▲"
	default:
		return " "
	}
}

// ToolIcon returns an icon for a tool name.
func ToolIcon(name string) string {
	switch name {
	case "read_file":
		return "📖"
	case "write_file":
		return "✍️"
	case "run_command":
		return "▶"
	case "grep_search":
		return "🔍"
	case "list_dir":
		return "📁"
	default:
		return "🔧"
	}
}
