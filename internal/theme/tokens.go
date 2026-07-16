// Package theme is the single source of truth for CodeForge visual tokens.
// Default palette mirrors Grok Build TUI "GrokNight" (neutral dark + magenta accent).
package theme

import (
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
)

// Tokens holds the full design-system palette (Grok color slots).
type Tokens struct {
	Name string

	// Background elevation
	BgBase     lipgloss.Color
	BgSurface  lipgloss.Color
	BgElevated lipgloss.Color
	BgOverlay  lipgloss.Color
	BgLight    lipgloss.Color // user prompt band (Grok bg_light)

	// Borders
	BorderDim       lipgloss.Color
	BorderActive    lipgloss.Color
	BorderGlow      lipgloss.Color
	PromptBorder    lipgloss.Color
	SelectionBorder lipgloss.Color

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

	// Scrollbar (Grok scrollbar_*)
	ScrollbarBg lipgloss.Color
	ScrollbarFg lipgloss.Color

	// Markdown accents (md_*) — used for glamour / inline code hints
	MdHeading lipgloss.Color
	MdLink    lipgloss.Color
	MdCode    lipgloss.Color
	MdCodeBg  lipgloss.Color
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
		SelectionBorder: "#E879F9",
		TextPrimary: "#EDEDEF", TextSecondary: "#A1A1AA", TextMuted: "#71717A", TextDisabled: "#52525B",
		// Magenta family accents (GrokNight signature)
		AccentUser: "#E879F9", AccentAssistant: "#C4B5FD", AccentTool: "#A78BFA",
		AccentThinking: "#818CF8", AccentSystem: "#71717A", AccentPlan: "#F0ABFC",
		AccentRunning: "#F472B6",
		AccentAI: "#C4B5FD", AccentAgent: "#A78BFA", AccentFocus: "#E879F9",
		Success: "#4ADE80", Danger: "#FB7185", Warning: "#FBBF24", Info: "#60A5FA",
		DiffAddBg: "#052e16", DiffAddFg: "#4ADE80", DiffDelBg: "#450a0a", DiffDelFg: "#FB7185", DiffCtxFg: "#71717A",
		ScrollbarBg: "#1C1C21", ScrollbarFg: "#52525B",
		MdHeading: "#E879F9", MdLink: "#60A5FA", MdCode: "#C4B5FD", MdCodeBg: "#1C1C21",
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
		SelectionBorder: "#22D3EE",
		TextPrimary: "#E6EDF3", TextSecondary: "#8B98A8", TextMuted: "#576273", TextDisabled: "#384250",
		AccentUser: "#38BDF8", AccentAssistant: "#22D3EE", AccentTool: "#A78BFA",
		AccentThinking: "#818CF8", AccentSystem: "#576273", AccentPlan: "#22D3EE", AccentRunning: "#F472B6",
		AccentAI: "#22D3EE", AccentAgent: "#A78BFA", AccentFocus: "#F472B6",
		Success: "#34D399", Danger: "#FB7185", Warning: "#FBBF24", Info: "#60A5FA",
		DiffAddBg: "#0F2419", DiffAddFg: "#34D399", DiffDelBg: "#2A1215", DiffDelFg: "#FB7185", DiffCtxFg: "#576273",
		ScrollbarBg: "#161C26", ScrollbarFg: "#576273",
		MdHeading: "#22D3EE", MdLink: "#60A5FA", MdCode: "#A78BFA", MdCodeBg: "#161C26",
	}
}

// Light / GrokDay-style light theme.
func Light() Tokens {
	return GrokDay()
}

// GrokDay is the light theme for bright terminal backgrounds.
func GrokDay() Tokens {
	return Tokens{
		Name: "grokday",
		BgBase: "#FAFAFA", BgSurface: "#F4F4F5", BgElevated: "#E4E4E7", BgOverlay: "#D4D4D8",
		BgLight: "#F0F0F3",
		BorderDim: "#D4D4D8", BorderActive: "#C026D3", BorderGlow: "#A21CAF", PromptBorder: "#D4D4D8",
		SelectionBorder: "#C026D3",
		TextPrimary: "#18181B", TextSecondary: "#52525B", TextMuted: "#A1A1AA", TextDisabled: "#D4D4D8",
		AccentUser: "#C026D3", AccentAssistant: "#7C3AED", AccentTool: "#6D28D9",
		AccentThinking: "#4F46E5", AccentSystem: "#71717A", AccentPlan: "#A21CAF", AccentRunning: "#DB2777",
		AccentAI: "#7C3AED", AccentAgent: "#6D28D9", AccentFocus: "#C026D3",
		Success: "#16A34A", Danger: "#E11D48", Warning: "#D97706", Info: "#2563EB",
		DiffAddBg: "#DCFCE7", DiffAddFg: "#15803D", DiffDelBg: "#FEE2E2", DiffDelFg: "#BE123C", DiffCtxFg: "#A1A1AA",
		ScrollbarBg: "#E4E4E7", ScrollbarFg: "#A1A1AA",
		MdHeading: "#C026D3", MdLink: "#2563EB", MdCode: "#7C3AED", MdCodeBg: "#F4F4F5",
	}
}

// TokyoNight blue-tinted dark (truecolor preferred).
func TokyoNight() Tokens {
	return Tokens{
		Name: "tokyonight",
		BgBase: "#1A1B26", BgSurface: "#1F2335", BgElevated: "#24283B", BgOverlay: "#292E42",
		BgLight: "#222436",
		BorderDim: "#3B4261", BorderActive: "#7AA2F7", BorderGlow: "#BB9AF7", PromptBorder: "#3B4261",
		SelectionBorder: "#BB9AF7",
		TextPrimary: "#C0CAF5", TextSecondary: "#A9B1D6", TextMuted: "#565F89", TextDisabled: "#414868",
		AccentUser: "#BB9AF7", AccentAssistant: "#7DCFFF", AccentTool: "#7AA2F7",
		AccentThinking: "#9D7CD8", AccentSystem: "#565F89", AccentPlan: "#BB9AF7", AccentRunning: "#FF9E64",
		AccentAI: "#7DCFFF", AccentAgent: "#7AA2F7", AccentFocus: "#BB9AF7",
		Success: "#9ECE6A", Danger: "#F7768E", Warning: "#E0AF68", Info: "#7AA2F7",
		DiffAddBg: "#1E2A1E", DiffAddFg: "#9ECE6A", DiffDelBg: "#2A1E22", DiffDelFg: "#F7768E", DiffCtxFg: "#565F89",
		ScrollbarBg: "#24283B", ScrollbarFg: "#565F89",
		MdHeading: "#BB9AF7", MdLink: "#7AA2F7", MdCode: "#7DCFFF", MdCodeBg: "#24283B",
	}
}

// RosePineMoon muted dark palette with mauve accents (truecolor preferred).
func RosePineMoon() Tokens {
	return Tokens{
		Name: "rosepine",
		BgBase: "#232136", BgSurface: "#2A273F", BgElevated: "#393552", BgOverlay: "#44415A",
		BgLight: "#2A273F",
		BorderDim: "#6E6A86", BorderActive: "#C4A7E7", BorderGlow: "#EBBCBA", PromptBorder: "#6E6A86",
		SelectionBorder: "#C4A7E7",
		TextPrimary: "#E0DEF4", TextSecondary: "#908CAA", TextMuted: "#6E6A86", TextDisabled: "#575279",
		AccentUser: "#C4A7E7", AccentAssistant: "#9CCFD8", AccentTool: "#EBBCBA",
		AccentThinking: "#C4A7E7", AccentSystem: "#6E6A86", AccentPlan: "#F6C177", AccentRunning: "#EA9A97",
		AccentAI: "#9CCFD8", AccentAgent: "#EBBCBA", AccentFocus: "#C4A7E7",
		Success: "#3E8FB0", Danger: "#EB6F92", Warning: "#F6C177", Info: "#9CCFD8",
		DiffAddBg: "#1F2A2E", DiffAddFg: "#9CCFD8", DiffDelBg: "#2E1F28", DiffDelFg: "#EB6F92", DiffCtxFg: "#6E6A86",
		ScrollbarBg: "#393552", ScrollbarFg: "#6E6A86",
		MdHeading: "#C4A7E7", MdLink: "#9CCFD8", MdCode: "#F6C177", MdCodeBg: "#2A273F",
	}
}

// OscuraMidnight deep dark base with purple accents (truecolor preferred).
func OscuraMidnight() Tokens {
	return Tokens{
		Name: "oscura",
		BgBase: "#0B0B0F", BgSurface: "#121218", BgElevated: "#1A1A22", BgOverlay: "#22222C",
		BgLight: "#14141C",
		BorderDim: "#2E2E3A", BorderActive: "#A78BFA", BorderGlow: "#C4B5FD", PromptBorder: "#2E2E3A",
		SelectionBorder: "#A78BFA",
		TextPrimary: "#E8E6F0", TextSecondary: "#9B97B0", TextMuted: "#6B6780", TextDisabled: "#4A4660",
		AccentUser: "#A78BFA", AccentAssistant: "#C4B5FD", AccentTool: "#8B5CF6",
		AccentThinking: "#818CF8", AccentSystem: "#6B6780", AccentPlan: "#DDD6FE", AccentRunning: "#C084FC",
		AccentAI: "#C4B5FD", AccentAgent: "#8B5CF6", AccentFocus: "#A78BFA",
		Success: "#4ADE80", Danger: "#F87171", Warning: "#FBBF24", Info: "#818CF8",
		DiffAddBg: "#0A1F14", DiffAddFg: "#4ADE80", DiffDelBg: "#2A1010", DiffDelFg: "#F87171", DiffCtxFg: "#6B6780",
		ScrollbarBg: "#1A1A22", ScrollbarFg: "#4A4660",
		MdHeading: "#A78BFA", MdLink: "#818CF8", MdCode: "#C4B5FD", MdCodeBg: "#1A1A22",
	}
}

// MinimalTokens returns terminal-native 16-color palette (no truecolor chrome).
// Used by --minimal: ignores user theme settings.
func MinimalTokens() Tokens {
	return Tokens{
		Name: "minimal",
		// Empty bg = terminal default (no forced paint)
		BgBase: "", BgSurface: "", BgElevated: "", BgOverlay: "",
		BgLight: "",
		BorderDim: "8", BorderActive: "5", BorderGlow: "13", PromptBorder: "8",
		SelectionBorder: "5",
		TextPrimary: "15", TextSecondary: "7", TextMuted: "8", TextDisabled: "8",
		AccentUser: "5", AccentAssistant: "6", AccentTool: "4",
		AccentThinking: "4", AccentSystem: "8", AccentPlan: "5", AccentRunning: "3",
		AccentAI: "6", AccentAgent: "4", AccentFocus: "5",
		Success: "2", Danger: "1", Warning: "3", Info: "4",
		DiffAddBg: "", DiffAddFg: "2", DiffDelBg: "", DiffDelFg: "1", DiffCtxFg: "8",
		ScrollbarBg: "", ScrollbarFg: "8",
		MdHeading: "5", MdLink: "4", MdCode: "6", MdCodeBg: "",
	}
}

// ThemeOption describes a selectable built-in theme for the picker.
type ThemeOption struct {
	Name        string
	Aliases     []string
	Description string
	Truecolor   bool // hide on non-truecolor terminals
	Factory     func() Tokens
}

// BuiltInThemes lists all themes for /theme picker (excluding auto/minimal).
func BuiltInThemes() []ThemeOption {
	return []ThemeOption{
		{Name: "groknight", Aliases: []string{"grok-night", "dark", "grok", "default"}, Description: "Neutral dark + magenta (default)", Truecolor: false, Factory: GrokNight},
		{Name: "grokday", Aliases: []string{"grok-day", "light", "day"}, Description: "Light theme for bright backgrounds", Truecolor: false, Factory: GrokDay},
		{Name: "tokyonight", Aliases: []string{"tokyo-night", "tokyo"}, Description: "Blue-tinted dark (Tokyo Night)", Truecolor: true, Factory: TokyoNight},
		{Name: "rosepine", Aliases: []string{"rose-pine", "rosepine-moon", "rose-pine-moon"}, Description: "Muted mauve Rosé Pine Moon", Truecolor: true, Factory: RosePineMoon},
		{Name: "oscura", Aliases: []string{"oscura-midnight"}, Description: "Deep dark + purple accents", Truecolor: true, Factory: OscuraMidnight},
		{Name: "aurora", Aliases: []string{}, Description: "Legacy CodeForge cyan", Truecolor: false, Factory: Aurora},
	}
}

var (
	mu      sync.RWMutex
	current = GrokNight()
	motion  = true
	compact = false
	minimal = false
	// auto mode: follow system light/dark
	autoMode       = false
	autoDarkName   = "groknight"
	autoLightName  = "grokday"
	// theme order for cycling (excludes auto)
	themeCycle = []func() Tokens{GrokNight, GrokDay, TokyoNight, RosePineMoon, OscuraMidnight, Aurora}
)

// Current returns the active theme tokens.
func Current() Tokens {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// Set replaces the active theme (applies quantization if needed).
func Set(t Tokens) {
	mu.Lock()
	current = QuantizeTokens(t)
	autoMode = false
	mu.Unlock()
}

// SetRaw sets tokens without clearing auto mode (used by auto resolve).
func setRaw(t Tokens) {
	current = QuantizeTokens(t)
}

// Name returns active theme name (or "auto" when auto mode is on).
func Name() string {
	mu.RLock()
	defer mu.RUnlock()
	if autoMode {
		return "auto→" + current.Name
	}
	return current.Name
}

// DisplayName returns the base theme name without auto prefix.
func DisplayName() string {
	return Current().Name
}

// IsAuto reports whether auto system appearance mode is active.
func IsAuto() bool {
	mu.RLock()
	defer mu.RUnlock()
	return autoMode
}

// Cycle advances to the next built-in theme and returns its name.
func Cycle() string {
	mu.Lock()
	defer mu.Unlock()
	autoMode = false
	name := current.Name
	idx := 0
	for i, f := range themeCycle {
		if f().Name == name {
			idx = (i + 1) % len(themeCycle)
			break
		}
	}
	setRaw(themeCycle[idx]())
	return current.Name
}

// SetByName applies a theme by config name.
// Supports: groknight, grokday, tokyonight, rosepine, oscura, aurora, auto, system, minimal.
func SetByName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "auto", "system":
		EnableAuto()
		return true
	case "minimal":
		SetMinimal(true)
		return true
	}
	for _, opt := range BuiltInThemes() {
		if n == opt.Name {
			Set(opt.Factory())
			return true
		}
		for _, a := range opt.Aliases {
			if n == a {
				Set(opt.Factory())
				return true
			}
		}
	}
	return false
}

// ThemeNames lists selectable themes (for help text).
func ThemeNames() []string {
	return []string{"groknight", "grokday", "tokyonight", "rosepine", "oscura", "aurora", "auto"}
}

// ThemeNamesForPicker returns themes suitable for the current terminal color level.
func ThemeNamesForPicker() []ThemeOption {
	opts := BuiltInThemes()
	level := DetectColorLevel()
	if level >= ColorTrue {
		// include auto as synthetic option handled by caller
		return opts
	}
	// Hide truecolor-only themes on weak terminals
	out := make([]ThemeOption, 0, len(opts))
	for _, o := range opts {
		if !o.Truecolor {
			out = append(out, o)
		}
	}
	return out
}

// MotionEnabled reports whether animations should run.
func MotionEnabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return motion && !minimal
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

// MinimalMode reports --minimal chrome-free mode.
func MinimalMode() bool {
	mu.RLock()
	defer mu.RUnlock()
	return minimal
}

// SetMinimal enables terminal-native 16-color mode (no chrome themes).
func SetMinimal(on bool) {
	mu.Lock()
	minimal = on
	if on {
		autoMode = false
		current = MinimalTokens()
		motion = false
	}
	mu.Unlock()
}

// SetAutoMapping sets which themes auto mode maps to for dark/light.
func SetAutoMapping(dark, light string) {
	mu.Lock()
	if dark != "" {
		autoDarkName = dark
	}
	if light != "" {
		autoLightName = light
	}
	mu.Unlock()
}

// EnableAuto turns on system appearance following.
func EnableAuto() {
	mu.Lock()
	autoMode = true
	minimal = false
	mu.Unlock()
	ResolveAuto()
}

// ResolveAuto re-detects system appearance and applies dark/light theme.
func ResolveAuto() {
	mu.Lock()
	if !autoMode {
		mu.Unlock()
		return
	}
	darkName := autoDarkName
	lightName := autoLightName
	mu.Unlock()

	light := DetectSystemLight()
	name := darkName
	if light {
		name = lightName
	}
	// Apply without clearing autoMode
	mu.Lock()
	defer mu.Unlock()
	autoMode = true
	for _, opt := range BuiltInThemes() {
		if opt.Name == name {
			setRaw(opt.Factory())
			return
		}
		for _, a := range opt.Aliases {
			if a == name {
				setRaw(opt.Factory())
				return
			}
		}
	}
	if light {
		setRaw(GrokDay())
	} else {
		setRaw(GrokNight())
	}
}

// InitFromEnv applies CODEFORGE_THEME, CODEFORGE_NO_MOTION, CODEFORGE_COMPACT, CODEFORGE_MINIMAL.
// Accessibility (Phase 9): NO_COLOR, CODEFORGE_REDUCE_MOTION / prefers-reduced-motion.
func InitFromEnv() {
	// Accessibility first
	if os.Getenv("NO_COLOR") != "" {
		// monochrome + no motion; force color level none via CODEFORGE_COLOR if unset
		if os.Getenv("CODEFORGE_COLOR") == "" {
			_ = os.Setenv("CODEFORGE_COLOR", "none")
			ResetColorLevelCache()
		}
		SetMotion(false)
	}
	if v := os.Getenv("CODEFORGE_NO_MOTION"); v == "1" || strings.EqualFold(v, "true") {
		SetMotion(false)
	}
	// Common a11y envs (GNOME/KDE sometimes set these)
	if v := os.Getenv("CODEFORGE_REDUCE_MOTION"); v == "1" || strings.EqualFold(v, "true") {
		SetMotion(false)
	}
	if v := os.Getenv("PREFERS_REDUCED_MOTION"); v == "1" || strings.EqualFold(v, "reduce") || strings.EqualFold(v, "true") {
		SetMotion(false)
	}
	if v := os.Getenv("CODEFORGE_COMPACT"); v == "1" || strings.EqualFold(v, "true") {
		SetCompact(true)
	}
	// SSH heuristic: slow links → compact + no motion (opt-in via CODEFORGE_SSH_TUNE=1)
	if v := os.Getenv("CODEFORGE_SSH_TUNE"); v == "1" || strings.EqualFold(v, "true") {
		if os.Getenv("SSH_CONNECTION") != "" || os.Getenv("SSH_TTY") != "" {
			SetMotion(false)
			SetCompact(true)
		}
	}
	if v := os.Getenv("CODEFORGE_MINIMAL"); v == "1" || strings.EqualFold(v, "true") {
		SetMinimal(true)
		return
	}
	// NO_COLOR without explicit theme → minimal chrome-free palette
	if os.Getenv("NO_COLOR") != "" && os.Getenv("CODEFORGE_THEME") == "" {
		SetMinimal(true)
		return
	}
	if dark := os.Getenv("CODEFORGE_AUTO_DARK"); dark != "" {
		SetAutoMapping(dark, os.Getenv("CODEFORGE_AUTO_LIGHT"))
	} else if light := os.Getenv("CODEFORGE_AUTO_LIGHT"); light != "" {
		SetAutoMapping("", light)
	}
	name := os.Getenv("CODEFORGE_THEME")
	if name == "" {
		name = "groknight"
	}
	SetByName(name)
	// YAML overrides only when not minimal/auto
	if !MinimalMode() && !IsAuto() {
		if t, err := LoadFromFile(""); err == nil && t != nil {
			Set(*t)
		}
	}
}

// ApplyFromConfig applies theme name from config after env (config wins if set).
func ApplyFromConfig(themeName string, compactMode bool, autoDark, autoLight string) {
	if MinimalMode() {
		return
	}
	if compactMode {
		SetCompact(true)
	}
	if autoDark != "" || autoLight != "" {
		SetAutoMapping(autoDark, autoLight)
	}
	if themeName != "" {
		SetByName(themeName)
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
	case "run_command", "run_terminal_command":
		return "▶"
	case "grep_search", "grep", "codebase_search":
		return "◈"
	case "glob_file_search", "glob", "find_files":
		return "⬡"
	case "list_dir", "list_directory":
		return "▣"
	case "github":
		return "⌘"
	case "search_replace", "edit_file", "apply_patch":
		return "▤"
	case "diagnostics":
		return "◎"
	case "research", "web_search":
		return "◉"
	case "fetch_url", "web_fetch":
		return "↗"
	case "memory_search", "memory_write":
		return "▣"
	case "spawn_subagent":
		return "◎"
	case "ask_user_question", "ask_user":
		return "？"
	case "todo_write":
		return "☑"
	case "write_plan", "enter_plan_mode", "exit_plan_mode":
		return "◈"
	default:
		return "◆"
	}
}

// GlamourStyleName returns the glamour standard style for the active theme.
func GlamourStyleName() string {
	if MinimalMode() {
		return "notty"
	}
	switch DisplayName() {
	case "grokday", "light":
		return "light"
	case "tokyonight":
		return "dracula"
	case "rosepine", "oscura", "groknight", "aurora":
		return "dark"
	default:
		return "dark"
	}
}
