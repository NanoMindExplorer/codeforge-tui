package pager

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/theme"
)

// MergeConfigUI overlays config.yaml [ui] fields onto pager config.
func MergeConfigUI(pg Config, cfg *config.Config) Config {
	if cfg == nil {
		return pg
	}
	u := cfg.UI
	if u.VimMode {
		t := true
		pg.UI.VimMode = &t
	}
	if u.CompactMode {
		t := true
		pg.UI.CompactMode = &t
	}
	if u.SimpleMode != nil {
		pg.UI.SimpleMode = u.SimpleMode
	}
	if u.ShowThinkingBlocks != nil {
		pg.UI.ShowThinkingBlocks = u.ShowThinkingBlocks
	}
	if u.MaxThoughtsWidth != nil {
		pg.UI.MaxThoughtsWidth = u.MaxThoughtsWidth
	}
	if u.GroupToolVerbs != nil {
		pg.UI.GroupToolVerbs = u.GroupToolVerbs
	}
	if u.ScreenMode != "" {
		pg.UI.ScreenMode = u.ScreenMode
	}
	if u.ScrollSpeed != nil {
		pg.UI.ScrollSpeed = u.ScrollSpeed
	}
	if u.ScrollMode != "" {
		pg.UI.ScrollMode = u.ScrollMode
	}
	if u.ScrollLines != nil {
		pg.UI.ScrollLines = u.ScrollLines
	}
	if u.InvertScroll != nil {
		pg.UI.InvertScroll = u.InvertScroll
	}
	if u.DefaultSelectedPermission != "" {
		pg.UI.DefaultSelectedPermission = u.DefaultSelectedPermission
	}
	if u.RememberToolApprovals != nil {
		pg.UI.RememberToolApprovals = u.RememberToolApprovals
	}
	return pg
}

// Apply pushes pager settings into theme + runtime globals.
func Apply(c Config) {
	SetGlobal(c)

	// Layout override (-1 = leave base mode matrix)
	lo := &theme.Layout{
		OuterVPad: -1, OuterHPadLeft: -1, OuterHPadRight: -1,
		BlockPadLeft: -1, BlockPadRight: -1, PromptPadV: -1,
	}
	if c.Layout.OuterVPad != nil {
		lo.OuterVPad = *c.Layout.OuterVPad
	}
	if c.Layout.OuterHPadLeft != nil {
		lo.OuterHPadLeft = *c.Layout.OuterHPadLeft
	}
	if c.Layout.OuterHPadRight != nil {
		lo.OuterHPadRight = *c.Layout.OuterHPadRight
	}
	if c.Layout.BlockPadLeft != nil {
		lo.BlockPadLeft = *c.Layout.BlockPadLeft
	}
	if c.Layout.BlockPadRight != nil {
		lo.BlockPadRight = *c.Layout.BlockPadRight
	}
	theme.SetLayoutOverride(lo)

	// Compact / minimal from UI / terminal
	if c.UI.CompactMode != nil {
		theme.SetCompact(*c.UI.CompactMode)
	}
	if c.Terminal.Minimal != nil && *c.Terminal.Minimal {
		theme.SetMinimal(true)
	}
	if c.UI.ScreenMode == "minimal" {
		theme.SetMinimal(true)
	}

	// Scrollbar colors
	tok := theme.Current()
	if c.Scrollbar.Bg != "" && c.Scrollbar.Bg != "none" {
		tok.ScrollbarBg = lipgloss.Color(c.Scrollbar.Bg)
	}
	if c.Scrollbar.Fg != "" && c.Scrollbar.Fg != "none" {
		tok.ScrollbarFg = lipgloss.Color(c.Scrollbar.Fg)
	}
	theme.SetTokens(tok)

	// Animation FPS
	theme.SetAnimationFPS(c.AnimationFPS())
}

// ApplyFromWorkdir loads then Apply.
func ApplyFromWorkdir(workdir string) Config {
	c := Load(workdir)
	Apply(c)
	return c
}
