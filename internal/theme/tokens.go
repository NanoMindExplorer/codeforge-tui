// Package theme is the single source of truth for CodeForge visual tokens.
// Default palette mirrors Grok Build TUI "GrokNight" (neutral dark + magenta accent).
package theme

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// Tokens holds the full design-system palette.
type Tokens struct {
	Name string

	// Background elevation
	BgBase     lipgloss.Color
	BgSurface  lipgloss.Color
	BgElevated lipgloss.Color
	BgOverlay  lipgloss.Color
	BgLight    lipgloss.Color // user prompt band (Grok bg_light)

	// Borders
	BorderDim    lipgloss.Color
	BorderActive lipgloss.Color
	BorderGlow   lipgloss.Color
	PromptBorder lipgloss.Color

	// Text hierarchy
	TextPrimary   lipgloss.Color
	TextSecondary lipgloss.Color
	TextMuted     lipgloss.Color
	TextDisabled  lipgloss.Color

	// Role accents (Grok slots)
	AccentUser      lipgloss.Color // user / cursor (magenta)
	AccentAssistant lipgloss.Color // AI replies
	AccentTool      lipgloss.Color // tool calls
	AccentThinking  lipgloss.Color
	AccentSystem    lipgloss.Color
	AccentPlan      lipgloss.Color
	AccentRunning   lipgloss.Color
	// Aliases used by existing code
	AccentAI    lipgloss.Color
	AccentAgent lipgloss.Color
	AccentFocus lipgloss.Color

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

// GrokNight is the Grok 4.5 / Grok Build default dark theme.
// Neutral grays + magenta user accent (survives 256-color quantization).
func GrokNight() Tokens {
	t := Tokens{
		Name: "groknight",
		// Neutral dark base (Grok-like)
		BgBase: "#0D0D0F", BgSurface: "#141417", BgElevated: "#1C1C21", BgOverlay: "#25252B",
		BgLight: "#1A1A1F",
		BorderDim: "#2A2A32", BorderActive: "#C084FC", BorderGlow: "#E879F9", PromptBorder: "#3F3F46",
		TextPrimary: "#EDEDEF", TextSecondary: "#A1A1AA", TextMuted: "#71717A", TextDisabled: "#52525B",
		// Magenta family accents (GrokNight signature)
		AccentUser: "#E879F9", AccentAssistant: "#C4B5FD", AccentTool: "#A78BFA",
		AccentThinking: "#818CF8", AccentSystem: "#71717A", AccentPlan: "#F0ABFC",
		AccentRunning: "#F472B6",
		AccentAI: "#C4B5FD", AccentAgent: "#A78BFA", AccentFocus: "#E879F9",
		Success: "#4ADE80", Danger: "#FB7185", Warning: "#FBBF24", Info: "#60A5FA",
		DiffAddBg: "#052e16", DiffAddFg: "#4ADE80", DiffDelBg: "#450a0a", DiffDelFg: "#FB7185", DiffCtxFg: "#71717A",
	}
	return t
}

// Aurora is the legacy CodeForge cyan theme (kept for /theme).
func Aurora() Tokens {
	return Tokens{
		Name: "aurora",
		BgBase: "#0A0E14", BgSurface: "#10151C", BgElevated: "#161C26", BgOverlay: "#1C2430",
		BgLight: "#121820",
		BorderDim: "#232B38", BorderActive: "#22D3EE", BorderGlow: "#A78BFA", PromptBorder: "#232B38",
		TextPrimary: "#E6EDF3", TextSecondary: "#8B98A8", TextMuted: "#576273", TextDisabled: "#384250",
		AccentUser: "#38BDF8", AccentAssistant: "#22D3EE", AccentTool: "#A78BFA",
		AccentThinking: "#818CF8", AccentSystem: "#576273", AccentPlan: "#22D3EE", AccentRunning: "#F472B6",
		AccentAI: "#22D3EE", AccentAgent: "#A78BFA", AccentFocus: "#F472B6",
		Success: "#34D399", Danger: "#FB7185", Warning: "#FBBF24", Info: "#60A5FA",
		DiffAddBg: "#0F2419", DiffAddFg: "#34D399", DiffDelBg: "#2A1215", DiffDelFg: "#FB7185", DiffCtxFg: "#576273",
	}
}

// Light / GrokDay-style light theme.
func Light() Tokens {
	return Tokens{
		Name: "grokday",
		BgBase: "#FAFAFA", BgSurface: "#F4F4F5", BgElevated: "#E4E4E7", BgOverlay: "#D4D4D8",
		BgLight: "#F0F0F3",
		BorderDim: "#D4D4D8", BorderActive: "#C026D3", BorderGlow: "#A21CAF", PromptBorder: "#D4D4D8",
		TextPrimary: "#18181B", TextSecondary: "#52525B", TextMuted: "#A1A1AA", TextDisabled: "#D4D4D8",
		AccentUser: "#C026D3", AccentAssistant: "#7C3AED", AccentTool: "#6D28D9",
		AccentThinking: "#4F46E5", AccentSystem: "#71717A", AccentPlan: "#A21CAF", AccentRunning: "#DB2777",
		AccentAI: "#7C3AED", AccentAgent: "#6D28D9", AccentFocus: "#C026D3",
		Success: "#16A34A", Danger: "#E11D48", Warning: "#D97706", Info: "#2563EB",
		DiffAddBg: "#DCFCE7", DiffAddFg: "#15803D", DiffDelBg: "#FEE2E2", DiffDelFg: "#BE123C", DiffCtxFg: "#A1A1AA",
	}
}

// TokyoNight blue-tinted dark.
func TokyoNight() Tokens {
	return Tokens{
		Name: "tokyonight",
		BgBase: "#1A1B26", BgSurface: "#1F2335", BgElevated: "#24283B", BgOverlay: "#292E42",
		BgLight: "#222436",
		BorderDim: "#3B4261", BorderActive: "#7AA2F7", BorderGlow: "#BB9AF7", PromptBorder: "#3B4261",
		TextPrimary: "#C0CAF5", TextSecondary: "#A9B1D6", TextMuted: "#565F89", TextDisabled: "#414868",
		AccentUser: "#BB9AF7", AccentAssistant: "#7DCFFF", AccentTool: "#7AA2F7",
		AccentThinking: "#9D7CD8", AccentSystem: "#565F89", AccentPlan: "#BB9AF7", AccentRunning: "#FF9E64",
		AccentAI: "#7DCFFF", AccentAgent: "#7AA2F7", AccentFocus: "#BB9AF7",
		Success: "#9ECE6A", Danger: "#F7768E", Warning: "#E0AF68", Info: "#7AA2F7",
		DiffAddBg: "#1E2A1E", DiffAddFg: "#9ECE6A", DiffDelBg: "#2A1E22", DiffDelFg: "#F7768E", DiffCtxFg: "#565F89",
	}
}

var (
	mu      sync.RWMutex
	current = GrokNight()
	motion  = true
	compact = false
	// theme order for cycling
	themeCycle = []func() Tokens{GrokNight, Aurora, TokyoNight, Light}
)

// Current returns the active theme tokens.
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

// Name returns active theme name.
func Name() string {
	return Current().Name
}

// Cycle advances to the next built-in theme and returns its name.
func Cycle() string {
	mu.Lock()
	defer mu.Unlock()
	name := current.Name
	idx := 0
	for i, f := range themeCycle {
		if f().Name == name {
			idx = (i + 1) % len(themeCycle)
			break
		}
	}
	current = themeCycle[idx]()
	return current.Name
}

// SetByName applies a theme by config name (groknight, aurora, tokyonight, light, day, dark).
func SetByName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "groknight", "grok-night", "dark", "grok", "default", "":
		Set(GrokNight())
	case "aurora":
		Set(Aurora())
	case "tokyonight", "tokyo-night", "tokyo":
		Set(TokyoNight())
	case "light", "day", "grokday", "grok-day":
		Set(Light())
	default:
		return false
	}
	return true
}

// ThemeNames lists selectable themes.
func ThemeNames() []string {
	return []string{"groknight", "aurora", "tokyonight", "grokday"}
}

// MotionEnabled reports whether animations should run.
func MotionEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return motion
}

// SetMotion enables/disables animations.
func SetMotion(on bool) {
	mu.Lock()
	motion = on
	mu.Unlock()
}

// CompactMode reports Grok-style compact padding.
func CompactMode() bool {
	mu.RLock()
	defer mu.RUnlock()
	return compact
}

// SetCompact toggles compact mode.
func SetCompact(on bool) {
	mu.Lock()
	compact = on
	mu.Unlock()
}

// ToggleCompact flips compact mode and returns new state.
func ToggleCompact() bool {
	mu.Lock()
	compact = !compact
	v := compact
	mu.Unlock()
	return v
}

// InitFromEnv applies CODEFORGE_THEME, CODEFORGE_NO_MOTION, CODEFORGE_COMPACT.
func InitFromEnv() {
	if v := os.Getenv("CODEFORGE_NO_MOTION"); v == "1" || strings.EqualFold(v, "true") {
		SetMotion(false)
	}
	if v := os.Getenv("CODEFORGE_COMPACT"); v == "1" || strings.EqualFold(v, "true") {
		SetCompact(true)
	}
	name := os.Getenv("CODEFORGE_THEME")
	if name == "" {
		name = "groknight"
	}
	SetByName(name)
	if t, err := LoadFromFile(""); err == nil && t != nil {
		Set(*t)
	}
}

// HasNerdFont detects Nerd Font availability.
func HasNerdFont() bool {
	if v := os.Getenv("NERD_FONT"); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	if v := os.Getenv("NERD_FONTS"); v != "" {
		return true
	}
	tp := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	return tp == "iTerm.app" || tp == "WezTerm" || tp == "ghostty"
}

// FileIcon returns a glyph for a file extension.
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
		default:
			return "󰈔"
		}
	}
	switch ext {
	case ".go":
		return "◆"
	case ".md":
		return "▣"
	default:
		return "●"
	}
}

// GitStatusGlyph returns a short glyph for git status.
func GitStatusGlyph(status string) string {
	switch status {
	case "M", "modified":
		return "●"
	case "A", "new", "??":
		return "○"
	case "D", "deleted":
		return "✕"
	case "staged":
		return "▲"
	default:
		return " "
	}
}

// ToolIcon returns an icon for a tool name (Grok-like diamond default).
func ToolIcon(name string) string {
	switch name {
	case "read_file":
		return "◇"
	case "write_file":
		return "◆"
	case "run_command":
		return "▶"
	case "grep_search", "codebase_search":
		return "◈"
	case "list_dir":
		return "▣"
	case "github":
		return "⌘"
	case "search_replace", "apply_patch":
		return "▤"
	case "diagnostics":
		return "◎"
	case "research":
		return "◉"
	case "fetch_url":
		return "↗"
	default:
		return "◆"
	}
}
