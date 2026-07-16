// Package pager loads Grok-compatible pager.toml (and pager.yaml) settings
// for scrollback layout, scrollbar, blocks, animation, and terminal chrome.
package pager

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config is the full pager surface (subset + extensions of Grok pager.toml).
type Config struct {
	Layout    LayoutConfig    `yaml:"layout" toml:"layout"`
	Scrollbar ScrollbarConfig `yaml:"scrollbar" toml:"scrollbar"`
	Scroll    ScrollConfig    `yaml:"scroll" toml:"scroll"`
	Display   DisplayConfig   `yaml:"display" toml:"display"`
	Animation AnimationConfig `yaml:"animation" toml:"animation"`
	Blocks    BlocksConfig    `yaml:"blocks" toml:"blocks"`
	Prompt    PromptConfig    `yaml:"prompt" toml:"prompt"`
	Todo      TodoConfig      `yaml:"todo" toml:"todo"`
	Terminal  TerminalConfig  `yaml:"terminal" toml:"terminal"`
	// DisablePlugins mirrors Grok top-level disable_plugins.
	DisablePlugins bool `yaml:"disable_plugins" toml:"disable_plugins"`
	// UI knobs often colocated with pager (also in config.yaml [ui]).
	UI UIKnobs `yaml:"ui" toml:"ui"`

	// Source path last loaded (for /pager status).
	Source string `yaml:"-" toml:"-"`
}

type LayoutConfig struct {
	OuterVPad      *int `yaml:"outer_vpad" toml:"outer_vpad"`
	OuterHPadLeft  *int `yaml:"outer_hpad_left" toml:"outer_hpad_left"`
	OuterHPadRight *int `yaml:"outer_hpad_right" toml:"outer_hpad_right"`
	BlockPadLeft   *int `yaml:"block_pad_left" toml:"block_pad_left"`
	BlockPadRight  *int `yaml:"block_pad_right" toml:"block_pad_right"`
}

type ScrollbarConfig struct {
	Enabled  *bool  `yaml:"enabled" toml:"enabled"`
	GapLeft  *int   `yaml:"gap_left" toml:"gap_left"`
	GapRight *int   `yaml:"gap_right" toml:"gap_right"`
	Bg       string `yaml:"scrollbar_bg" toml:"scrollbar_bg"`
	Fg       string `yaml:"scrollbar_fg" toml:"scrollbar_fg"`
}

type ScrollConfig struct {
	Margin           *int   `yaml:"margin" toml:"margin"`
	MinPageFraction  *int   `yaml:"min_page_fraction" toml:"min_page_fraction"`
	FollowIndicator  string `yaml:"follow_indicator" toml:"follow_indicator"` // center | none
	FollowAutoSelect *bool  `yaml:"follow_auto_select" toml:"follow_auto_select"`
	FollowByOverscroll *bool `yaml:"follow_by_overscroll" toml:"follow_by_overscroll"`
	AnchorOnFold     *bool  `yaml:"anchor_on_fold" toml:"anchor_on_fold"`
}

type DisplayConfig struct {
	StickyHeaders           *bool   `yaml:"sticky_headers" toml:"sticky_headers"`
	TabWidth                *int    `yaml:"tab_width" toml:"tab_width"`
	ExpandableIndicator     *bool   `yaml:"expandable_indicator" toml:"expandable_indicator"`
	ExpandableIndicatorChar string  `yaml:"expandable_indicator_char" toml:"expandable_indicator_char"`
	CollapsedAccentChar     string  `yaml:"collapsed_accent_char" toml:"collapsed_accent_char"`
	DimAccent               *float64 `yaml:"dim_accent" toml:"dim_accent"`
	LineUnderLastEntry      *bool   `yaml:"line_under_last_entry" toml:"line_under_last_entry"`
	SelectionButtons        *bool   `yaml:"selection_buttons" toml:"selection_buttons"`
}

type AnimationConfig struct {
	FPS      *int `yaml:"fps" toml:"fps"`
	WaveRows *int `yaml:"wave_rows" toml:"wave_rows"`
}

type BlocksConfig struct {
	Edit     EditBlockConfig     `yaml:"edit" toml:"edit"`
	Thinking ThinkingBlockConfig `yaml:"thinking" toml:"thinking"`
	Tool     ToolBlockConfig     `yaml:"tool" toml:"tool"`
	Execute  ExecuteBlockConfig  `yaml:"execute" toml:"execute"`
	Prompt   PromptBlockConfig   `yaml:"prompt" toml:"prompt"`
}

type EditBlockConfig struct {
	Indent            *bool  `yaml:"indent" toml:"indent"`
	VPad              *bool  `yaml:"vpad" toml:"vpad"`
	ExpandedByDefault *bool  `yaml:"expanded_by_default" toml:"expanded_by_default"`
	HunkSeparator     string `yaml:"hunk_separator" toml:"hunk_separator"`
	DualLineNumbers   *bool  `yaml:"dual_line_numbers" toml:"dual_line_numbers"`
	LineSummary       *bool  `yaml:"line_summary" toml:"line_summary"`
	Bg                string `yaml:"bg" toml:"bg"`
}

type ThinkingBlockConfig struct {
	AccentEnabled *bool `yaml:"accent_enabled" toml:"accent_enabled"`
	Animate       *bool `yaml:"animate" toml:"animate"`
	TruncateLines *int  `yaml:"truncate_lines" toml:"truncate_lines"`
	BgBlend       *int  `yaml:"bg_blend" toml:"bg_blend"`
	Header        *bool `yaml:"header" toml:"header"`
	HeaderBright  *bool `yaml:"header_bright" toml:"header_bright"`
}

type ToolBlockConfig struct {
	MutedCollapsed *bool  `yaml:"muted_collapsed" toml:"muted_collapsed"`
	DimDetails     *bool  `yaml:"dim_details" toml:"dim_details"`
	Bullet         string `yaml:"bullet" toml:"bullet"` // none|dot|small-circle|circle|small-triangle|triangle|diamond
}

type ExecuteBlockConfig struct {
	FirstLines             *int   `yaml:"first_lines" toml:"first_lines"`
	LastLines              *int   `yaml:"last_lines" toml:"last_lines"`
	AccentEnabled          *bool  `yaml:"accent_enabled" toml:"accent_enabled"`
	HeaderStyle            string `yaml:"header_style" toml:"header_style"` // shell|label
	MutedCommandCollapsed  *bool  `yaml:"muted_command_collapsed" toml:"muted_command_collapsed"`
}

type PromptBlockConfig struct {
	VPad       *bool  `yaml:"vpad" toml:"vpad"`
	Bg         string `yaml:"bg" toml:"bg"` // none|light|dark
	ShowPrefix *bool  `yaml:"show_prefix" toml:"show_prefix"`
	MinLines   *int   `yaml:"min_lines" toml:"min_lines"`
}

type PromptConfig struct {
	CollapseUnfocused *bool `yaml:"collapse_unfocused" toml:"collapse_unfocused"`
	MouseHover        *bool `yaml:"mouse_hover" toml:"mouse_hover"`
	ShowPrefix        *bool `yaml:"show_prefix" toml:"show_prefix"`
}

type TodoConfig struct {
	BadgeFormat string `yaml:"badge_format" toml:"badge_format"` // default|colon|comma
}

type TerminalConfig struct {
	AltScreen string `yaml:"alt_screen" toml:"alt_screen"` // auto|always|never
	Minimal   *bool  `yaml:"minimal" toml:"minimal"`
}

// UIKnobs are Grok [ui] settings that affect the pager session.
type UIKnobs struct {
	MaxThoughtsWidth          *int    `yaml:"max_thoughts_width" toml:"max_thoughts_width"`
	ShowThinkingBlocks        *bool   `yaml:"show_thinking_blocks" toml:"show_thinking_blocks"`
	GroupToolVerbs            *bool   `yaml:"group_tool_verbs" toml:"group_tool_verbs"`
	ScreenMode                string  `yaml:"screen_mode" toml:"screen_mode"` // minimal|fullscreen
	ScrollSpeed               *int    `yaml:"scroll_speed" toml:"scroll_speed"` // 1-100
	ScrollMode                string  `yaml:"scroll_mode" toml:"scroll_mode"`   // auto|wheel|trackpad
	ScrollLines               *int    `yaml:"scroll_lines" toml:"scroll_lines"`
	InvertScroll              *bool   `yaml:"invert_scroll" toml:"invert_scroll"`
	SimpleMode                *bool   `yaml:"simple_mode" toml:"simple_mode"`
	VimMode                   *bool   `yaml:"vim_mode" toml:"vim_mode"`
	CompactMode               *bool   `yaml:"compact_mode" toml:"compact_mode"`
	DefaultSelectedPermission string  `yaml:"default_selected_permission" toml:"default_selected_permission"`
	RememberToolApprovals     *bool   `yaml:"remember_tool_approvals" toml:"remember_tool_approvals"`
}

var (
	mu     sync.RWMutex
	global Config
	loaded bool
)

// Global returns the active pager config (defaults if never loaded).
func Global() Config {
	mu.RLock()
	defer mu.RUnlock()
	if !loaded {
		return Defaults()
	}
	return global
}

// SetGlobal installs config (tests / Apply).
func SetGlobal(c Config) {
	mu.Lock()
	global = c
	loaded = true
	mu.Unlock()
}

// Defaults match Grok pager.toml documented defaults.
func Defaults() Config {
	t := true
	f := false
	vpad1, hpad2, bpad2 := 1, 2, 2
	fps30, wave32 := 30, 32
	trunc3 := 3
	blend70 := 70
	first2, last3 := 2, 3
	tab4 := 4
	dim05 := 0.5
	speed50 := 50
	thoughts120 := 120
	return Config{
		Layout: LayoutConfig{
			OuterVPad: &vpad1, OuterHPadLeft: &hpad2, OuterHPadRight: &hpad2,
			BlockPadLeft: &bpad2, BlockPadRight: &bpad2,
		},
		Scrollbar: ScrollbarConfig{Enabled: &t},
		Scroll: ScrollConfig{
			FollowIndicator: "center", FollowAutoSelect: &t, FollowByOverscroll: &t, AnchorOnFold: &t,
		},
		Display: DisplayConfig{
			StickyHeaders: &t, TabWidth: &tab4, ExpandableIndicator: &t,
			ExpandableIndicatorChar: "›", CollapsedAccentChar: "❙", DimAccent: &dim05,
			LineUnderLastEntry: &f, SelectionButtons: &f,
		},
		Animation: AnimationConfig{FPS: &fps30, WaveRows: &wave32},
		Blocks: BlocksConfig{
			Edit: EditBlockConfig{
				Indent: &t, VPad: &f, ExpandedByDefault: &t, HunkSeparator: "…",
			},
			Thinking: ThinkingBlockConfig{
				AccentEnabled: &t, Animate: &t, TruncateLines: &trunc3, BgBlend: &blend70,
				Header: &t, HeaderBright: &f,
			},
			Tool: ToolBlockConfig{
				MutedCollapsed: &t, DimDetails: &t, Bullet: "diamond",
			},
			Execute: ExecuteBlockConfig{
				FirstLines: &first2, LastLines: &last3, AccentEnabled: &t,
				HeaderStyle: "label", MutedCommandCollapsed: &t,
			},
			Prompt: PromptBlockConfig{
				VPad: &t, Bg: "light", ShowPrefix: &t,
			},
		},
		Prompt: PromptConfig{CollapseUnfocused: &t, MouseHover: &t, ShowPrefix: &t},
		Todo:   TodoConfig{BadgeFormat: "default"},
		Terminal: TerminalConfig{AltScreen: "auto"},
		UI: UIKnobs{
			MaxThoughtsWidth: &thoughts120, ShowThinkingBlocks: &t, GroupToolVerbs: &t,
			ScreenMode: "fullscreen", ScrollSpeed: &speed50, ScrollMode: "auto",
			InvertScroll: &f, SimpleMode: &t,
		},
	}
}

// Load discovers pager.toml / pager.yaml from standard locations and merges over defaults.
// Priority (last wins): defaults < ~/.grok/pager.toml < ~/.codeforge/pager.toml
// < project .grok/pager.toml < project .codeforge/pager.toml
// Also reads env CODEFORGE_PAGER / GROK_PAGER path.
func Load(workdir string) Config {
	cfg := Defaults()
	home, _ := os.UserHomeDir()
	candidates := []string{}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".grok", "pager.toml"),
			filepath.Join(home, ".grok", "pager.yaml"),
			filepath.Join(home, ".codeforge", "pager.toml"),
			filepath.Join(home, ".codeforge", "pager.yaml"),
		)
	}
	if workdir != "" {
		candidates = append(candidates,
			filepath.Join(workdir, ".grok", "pager.toml"),
			filepath.Join(workdir, ".grok", "pager.yaml"),
			filepath.Join(workdir, ".codeforge", "pager.toml"),
			filepath.Join(workdir, ".codeforge", "pager.yaml"),
		)
	}
	if p := os.Getenv("CODEFORGE_PAGER"); p != "" {
		candidates = append(candidates, p)
	}
	if p := os.Getenv("GROK_PAGER"); p != "" {
		candidates = append(candidates, p)
	}

	var src string
	for _, path := range candidates {
		partial, err := loadFile(path)
		if err != nil {
			continue
		}
		cfg = merge(cfg, partial)
		src = path
	}
	cfg.Source = src
	SetGlobal(cfg)
	return cfg
}

func loadFile(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	low := strings.ToLower(path)
	if strings.HasSuffix(low, ".yaml") || strings.HasSuffix(low, ".yml") {
		// remap nested scrollback.* if present
		if err := yaml.Unmarshal(data, &c); err != nil {
			// try nested under scrollback
			var wrap struct {
				Scrollback struct {
					Layout    LayoutConfig    `yaml:"layout"`
					Scrollbar ScrollbarConfig `yaml:"scrollbar"`
					Scroll    ScrollConfig    `yaml:"scroll"`
					Display   DisplayConfig   `yaml:"display"`
					Blocks    BlocksConfig    `yaml:"blocks"`
				} `yaml:"scrollback"`
				Animation AnimationConfig `yaml:"animation"`
				Prompt    PromptConfig    `yaml:"prompt"`
				Todo      TodoConfig      `yaml:"todo"`
				Terminal  TerminalConfig  `yaml:"terminal"`
				UI        UIKnobs         `yaml:"ui"`
				DisablePlugins bool       `yaml:"disable_plugins"`
			}
			if err2 := yaml.Unmarshal(data, &wrap); err2 != nil {
				return Config{}, err
			}
			c.Layout = wrap.Scrollback.Layout
			c.Scrollbar = wrap.Scrollback.Scrollbar
			c.Scroll = wrap.Scrollback.Scroll
			c.Display = wrap.Scrollback.Display
			c.Blocks = wrap.Scrollback.Blocks
			c.Animation = wrap.Animation
			c.Prompt = wrap.Prompt
			c.Todo = wrap.Todo
			c.Terminal = wrap.Terminal
			c.UI = wrap.UI
			c.DisablePlugins = wrap.DisablePlugins
		}
		return c, nil
	}
	// TOML (Grok native)
	return parseTOML(string(data))
}

// Summary one-line for bootstrap.
func (c Config) Summary() string {
	if c.Source == "" {
		return "pager: defaults"
	}
	return fmt.Sprintf("pager: %s", c.Source)
}

// Effective helpers ------------------------------------------------

func (c Config) ScrollbarEnabled() bool {
	if c.Scrollbar.Enabled == nil {
		return true
	}
	return *c.Scrollbar.Enabled
}

func (c Config) StickyHeaders() bool {
	if c.Display.StickyHeaders == nil {
		return true
	}
	return *c.Display.StickyHeaders
}

func (c Config) ShowThinking() bool {
	if c.UI.ShowThinkingBlocks == nil {
		return true
	}
	return *c.UI.ShowThinkingBlocks
}

func (c Config) ToolBulletChar() string {
	return bulletChar(c.Blocks.Tool.Bullet)
}

func (c Config) ExpandableChar() string {
	if c.Display.ExpandableIndicatorChar != "" {
		return c.Display.ExpandableIndicatorChar
	}
	return "›"
}

func (c Config) CollapsedAccent() string {
	if c.Display.CollapsedAccentChar != "" {
		return c.Display.CollapsedAccentChar
	}
	return "❙"
}

func (c Config) FollowIndicator() string {
	if c.Display.StickyHeaders != nil {
		// no-op keep API
	}
	if c.Scroll.FollowIndicator == "" {
		return "center"
	}
	return c.Scroll.FollowIndicator
}

func (c Config) AnimationFPS() int {
	if c.Animation.FPS == nil {
		return 30
	}
	fps := *c.Animation.FPS
	if fps < 1 {
		fps = 1
	}
	if fps > 60 {
		fps = 60
	}
	return fps
}

func (c Config) InvertScroll() bool {
	return c.UI.InvertScroll != nil && *c.UI.InvertScroll
}

func (c Config) ScrollLines() int {
	if c.UI.ScrollLines == nil {
		return 0 // terminal default
	}
	n := *c.UI.ScrollLines
	if n < 1 {
		n = 1
	}
	if n > 10 {
		n = 10
	}
	return n
}

func (c Config) ScrollSpeedMult() float64 {
	// Grok: 50 = 1.0x, 1 = 0.1x, 100 = 6.0x
	sp := 50
	if c.UI.ScrollSpeed != nil {
		sp = *c.UI.ScrollSpeed
	}
	if sp < 1 {
		sp = 1
	}
	if sp > 100 {
		sp = 100
	}
	if sp <= 50 {
		return 0.1 + (float64(sp-1)/49.0)*0.9
	}
	return 1.0 + (float64(sp-50)/50.0)*5.0
}

func (c Config) MaxThoughtsWidth() int {
	if c.UI.MaxThoughtsWidth == nil {
		return 120
	}
	return *c.UI.MaxThoughtsWidth
}

func (c Config) ThinkingTruncateLines() int {
	if c.Blocks.Thinking.TruncateLines == nil {
		return 3
	}
	return *c.Blocks.Thinking.TruncateLines
}

func (c Config) ThinkingHeader() bool {
	if c.Blocks.Thinking.Header == nil {
		return true
	}
	return *c.Blocks.Thinking.Header
}

func (c Config) ThinkingAnimate() bool {
	if c.Blocks.Thinking.Animate == nil {
		return true
	}
	return *c.Blocks.Thinking.Animate
}

func (c Config) DiffExpandedDefault() bool {
	if c.Blocks.Edit.ExpandedByDefault == nil {
		return true
	}
	return *c.Blocks.Edit.ExpandedByDefault
}

func (c Config) TodoBadgeFormat() string {
	if c.Todo.BadgeFormat == "" {
		return "default"
	}
	return c.Todo.BadgeFormat
}

func (c Config) GroupToolVerbs() bool {
	if c.UI.GroupToolVerbs == nil {
		return true
	}
	return *c.UI.GroupToolVerbs
}

func bulletChar(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "diamond":
		return "◆"
	case "none":
		return ""
	case "dot":
		return "·"
	case "small-circle", "small_circle":
		return "•"
	case "circle":
		return "●"
	case "small-triangle", "small_triangle":
		return "▸"
	case "triangle":
		return "▶"
	default:
		return "◆"
	}
}

// --- merge helpers (non-nil overrides) ---

func merge(base, over Config) Config {
	// Use YAML round-trip for deep partial merge simplicity
	// Encode over as map, only non-zero fields... manual is safer for pointers.

	// Layout
	if over.Layout.OuterVPad != nil {
		base.Layout.OuterVPad = over.Layout.OuterVPad
	}
	if over.Layout.OuterHPadLeft != nil {
		base.Layout.OuterHPadLeft = over.Layout.OuterHPadLeft
	}
	if over.Layout.OuterHPadRight != nil {
		base.Layout.OuterHPadRight = over.Layout.OuterHPadRight
	}
	if over.Layout.BlockPadLeft != nil {
		base.Layout.BlockPadLeft = over.Layout.BlockPadLeft
	}
	if over.Layout.BlockPadRight != nil {
		base.Layout.BlockPadRight = over.Layout.BlockPadRight
	}
	// Scrollbar
	if over.Scrollbar.Enabled != nil {
		base.Scrollbar.Enabled = over.Scrollbar.Enabled
	}
	if over.Scrollbar.GapLeft != nil {
		base.Scrollbar.GapLeft = over.Scrollbar.GapLeft
	}
	if over.Scrollbar.GapRight != nil {
		base.Scrollbar.GapRight = over.Scrollbar.GapRight
	}
	if over.Scrollbar.Bg != "" {
		base.Scrollbar.Bg = over.Scrollbar.Bg
	}
	if over.Scrollbar.Fg != "" {
		base.Scrollbar.Fg = over.Scrollbar.Fg
	}
	// Scroll
	if over.Scroll.Margin != nil {
		base.Scroll.Margin = over.Scroll.Margin
	}
	if over.Scroll.MinPageFraction != nil {
		base.Scroll.MinPageFraction = over.Scroll.MinPageFraction
	}
	if over.Scroll.FollowIndicator != "" {
		base.Scroll.FollowIndicator = over.Scroll.FollowIndicator
	}
	if over.Scroll.FollowAutoSelect != nil {
		base.Scroll.FollowAutoSelect = over.Scroll.FollowAutoSelect
	}
	if over.Scroll.FollowByOverscroll != nil {
		base.Scroll.FollowByOverscroll = over.Scroll.FollowByOverscroll
	}
	if over.Scroll.AnchorOnFold != nil {
		base.Scroll.AnchorOnFold = over.Scroll.AnchorOnFold
	}
	// Display
	if over.Display.StickyHeaders != nil {
		base.Display.StickyHeaders = over.Display.StickyHeaders
	}
	if over.Display.TabWidth != nil {
		base.Display.TabWidth = over.Display.TabWidth
	}
	if over.Display.ExpandableIndicator != nil {
		base.Display.ExpandableIndicator = over.Display.ExpandableIndicator
	}
	if over.Display.ExpandableIndicatorChar != "" {
		base.Display.ExpandableIndicatorChar = over.Display.ExpandableIndicatorChar
	}
	if over.Display.CollapsedAccentChar != "" {
		base.Display.CollapsedAccentChar = over.Display.CollapsedAccentChar
	}
	if over.Display.DimAccent != nil {
		base.Display.DimAccent = over.Display.DimAccent
	}
	if over.Display.LineUnderLastEntry != nil {
		base.Display.LineUnderLastEntry = over.Display.LineUnderLastEntry
	}
	if over.Display.SelectionButtons != nil {
		base.Display.SelectionButtons = over.Display.SelectionButtons
	}
	// Animation
	if over.Animation.FPS != nil {
		base.Animation.FPS = over.Animation.FPS
	}
	if over.Animation.WaveRows != nil {
		base.Animation.WaveRows = over.Animation.WaveRows
	}
	// Blocks - merge each subsection field by field (key ones)
	base.Blocks = mergeBlocks(base.Blocks, over.Blocks)
	// Prompt / Todo / Terminal / UI
	if over.Prompt.CollapseUnfocused != nil {
		base.Prompt.CollapseUnfocused = over.Prompt.CollapseUnfocused
	}
	if over.Prompt.MouseHover != nil {
		base.Prompt.MouseHover = over.Prompt.MouseHover
	}
	if over.Prompt.ShowPrefix != nil {
		base.Prompt.ShowPrefix = over.Prompt.ShowPrefix
	}
	if over.Todo.BadgeFormat != "" {
		base.Todo.BadgeFormat = over.Todo.BadgeFormat
	}
	if over.Terminal.AltScreen != "" {
		base.Terminal.AltScreen = over.Terminal.AltScreen
	}
	if over.Terminal.Minimal != nil {
		base.Terminal.Minimal = over.Terminal.Minimal
	}
	if over.DisablePlugins {
		base.DisablePlugins = true
	}
	base.UI = mergeUI(base.UI, over.UI)
	return base
}

func mergeBlocks(base, over BlocksConfig) BlocksConfig {
	// Edit
	if over.Edit.Indent != nil {
		base.Edit.Indent = over.Edit.Indent
	}
	if over.Edit.VPad != nil {
		base.Edit.VPad = over.Edit.VPad
	}
	if over.Edit.ExpandedByDefault != nil {
		base.Edit.ExpandedByDefault = over.Edit.ExpandedByDefault
	}
	if over.Edit.HunkSeparator != "" {
		base.Edit.HunkSeparator = over.Edit.HunkSeparator
	}
	if over.Edit.DualLineNumbers != nil {
		base.Edit.DualLineNumbers = over.Edit.DualLineNumbers
	}
	if over.Edit.LineSummary != nil {
		base.Edit.LineSummary = over.Edit.LineSummary
	}
	if over.Edit.Bg != "" {
		base.Edit.Bg = over.Edit.Bg
	}
	// Thinking
	if over.Thinking.AccentEnabled != nil {
		base.Thinking.AccentEnabled = over.Thinking.AccentEnabled
	}
	if over.Thinking.Animate != nil {
		base.Thinking.Animate = over.Thinking.Animate
	}
	if over.Thinking.TruncateLines != nil {
		base.Thinking.TruncateLines = over.Thinking.TruncateLines
	}
	if over.Thinking.BgBlend != nil {
		base.Thinking.BgBlend = over.Thinking.BgBlend
	}
	if over.Thinking.Header != nil {
		base.Thinking.Header = over.Thinking.Header
	}
	if over.Thinking.HeaderBright != nil {
		base.Thinking.HeaderBright = over.Thinking.HeaderBright
	}
	// Tool
	if over.Tool.MutedCollapsed != nil {
		base.Tool.MutedCollapsed = over.Tool.MutedCollapsed
	}
	if over.Tool.DimDetails != nil {
		base.Tool.DimDetails = over.Tool.DimDetails
	}
	if over.Tool.Bullet != "" {
		base.Tool.Bullet = over.Tool.Bullet
	}
	// Execute
	if over.Execute.FirstLines != nil {
		base.Execute.FirstLines = over.Execute.FirstLines
	}
	if over.Execute.LastLines != nil {
		base.Execute.LastLines = over.Execute.LastLines
	}
	if over.Execute.AccentEnabled != nil {
		base.Execute.AccentEnabled = over.Execute.AccentEnabled
	}
	if over.Execute.HeaderStyle != "" {
		base.Execute.HeaderStyle = over.Execute.HeaderStyle
	}
	if over.Execute.MutedCommandCollapsed != nil {
		base.Execute.MutedCommandCollapsed = over.Execute.MutedCommandCollapsed
	}
	// Prompt block
	if over.Prompt.VPad != nil {
		base.Prompt.VPad = over.Prompt.VPad
	}
	if over.Prompt.Bg != "" {
		base.Prompt.Bg = over.Prompt.Bg
	}
	if over.Prompt.ShowPrefix != nil {
		base.Prompt.ShowPrefix = over.Prompt.ShowPrefix
	}
	if over.Prompt.MinLines != nil {
		base.Prompt.MinLines = over.Prompt.MinLines
	}
	return base
}

func mergeUI(base, over UIKnobs) UIKnobs {
	if over.MaxThoughtsWidth != nil {
		base.MaxThoughtsWidth = over.MaxThoughtsWidth
	}
	if over.ShowThinkingBlocks != nil {
		base.ShowThinkingBlocks = over.ShowThinkingBlocks
	}
	if over.GroupToolVerbs != nil {
		base.GroupToolVerbs = over.GroupToolVerbs
	}
	if over.ScreenMode != "" {
		base.ScreenMode = over.ScreenMode
	}
	if over.ScrollSpeed != nil {
		base.ScrollSpeed = over.ScrollSpeed
	}
	if over.ScrollMode != "" {
		base.ScrollMode = over.ScrollMode
	}
	if over.ScrollLines != nil {
		base.ScrollLines = over.ScrollLines
	}
	if over.InvertScroll != nil {
		base.InvertScroll = over.InvertScroll
	}
	if over.SimpleMode != nil {
		base.SimpleMode = over.SimpleMode
	}
	if over.VimMode != nil {
		base.VimMode = over.VimMode
	}
	if over.CompactMode != nil {
		base.CompactMode = over.CompactMode
	}
	if over.DefaultSelectedPermission != "" {
		base.DefaultSelectedPermission = over.DefaultSelectedPermission
	}
	if over.RememberToolApprovals != nil {
		base.RememberToolApprovals = over.RememberToolApprovals
	}
	return base
}

// parseTOML is a minimal TOML reader for Grok pager.toml tables.
func parseTOML(s string) (Config, error) {
	// Build nested map then convert known paths into Config via YAML
	root := map[string]any{}
	section := ""
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if strings.HasPrefix(trim, "[") && strings.HasSuffix(trim, "]") {
			section = strings.Trim(trim, "[]")
			continue
		}
		eq := strings.Index(trim, "=")
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(trim[:eq])
		val := strings.TrimSpace(trim[eq+1:])
		// strip comments
		if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		fullKey := key
		if section != "" {
			fullKey = section + "." + key
		}
		setNested(root, fullKey, parseTOMLValue(val))
	}
	// Remap scrollback.* to top-level for Config
	if sb, ok := root["scrollback"].(map[string]any); ok {
		for k, v := range sb {
			if _, exists := root[k]; !exists {
				root[k] = v
			}
		}
	}
	raw, err := yaml.Marshal(root)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func parseTOMLValue(v string) any {
	v = strings.TrimSpace(v)
	if (strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`)) ||
		(strings.HasPrefix(v, `'`) && strings.HasSuffix(v, `'`)) {
		return v[1 : len(v)-1]
	}
	switch strings.ToLower(v) {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	return v
}

func setNested(m map[string]any, path string, val any) {
	parts := strings.Split(path, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
}
