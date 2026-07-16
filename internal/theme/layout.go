package theme

// Layout padding mirrors Grok pager.toml scrollback.layout defaults.
// Overrides come from package pager via SetLayoutOverride.

// Layout holds outer/inner padding for chrome.
type Layout struct {
	OuterVPad      int // top/bottom viewport margin
	OuterHPadLeft  int
	OuterHPadRight int
	BlockPadLeft   int
	BlockPadRight  int
	PromptPadV     int // vertical padding inside prompt frame
}

// layoutOverride is set from pager.toml when non-nil fields are present.
var layoutOverride *Layout

// SetLayoutOverride applies pager.toml layout (pass nil to clear).
func SetLayoutOverride(l *Layout) {
	layoutOverride = l
}

// CurrentLayout returns padding matrix for full / compact / minimal + pager overrides.
func CurrentLayout() Layout {
	var base Layout
	if MinimalMode() {
		base = Layout{
			OuterVPad: 0, OuterHPadLeft: 0, OuterHPadRight: 0,
			BlockPadLeft: 1, BlockPadRight: 0, PromptPadV: 0,
		}
	} else if CompactMode() {
		base = Layout{
			OuterVPad: 0, OuterHPadLeft: 1, OuterHPadRight: 1,
			BlockPadLeft: 1, BlockPadRight: 1, PromptPadV: 0,
		}
	} else {
		// Grok defaults
		base = Layout{
			OuterVPad: 1, OuterHPadLeft: 2, OuterHPadRight: 2,
			BlockPadLeft: 2, BlockPadRight: 2, PromptPadV: 1,
		}
	}
	if layoutOverride != nil {
		// only apply positive/set values — override uses -1 as "unset"
		o := layoutOverride
		if o.OuterVPad >= 0 {
			base.OuterVPad = o.OuterVPad
		}
		if o.OuterHPadLeft >= 0 {
			base.OuterHPadLeft = o.OuterHPadLeft
		}
		if o.OuterHPadRight >= 0 {
			base.OuterHPadRight = o.OuterHPadRight
		}
		if o.BlockPadLeft >= 0 {
			base.BlockPadLeft = o.BlockPadLeft
		}
		if o.BlockPadRight >= 0 {
			base.BlockPadRight = o.BlockPadRight
		}
		if o.PromptPadV >= 0 {
			base.PromptPadV = o.PromptPadV
		}
	}
	// Grok: outer_hpad_left minimum 1 in non-minimal
	if !MinimalMode() && base.OuterHPadLeft < 1 {
		base.OuterHPadLeft = 1
	}
	return base
}

// PromptHeight estimates composer height in rows.
func PromptHeight() int {
	if MinimalMode() {
		return 3
	}
	if CompactMode() {
		return 4
	}
	return 6
}

// FooterHeight estimates footer + hints rows.
func FooterHeight() int {
	if MinimalMode() {
		return 1
	}
	if CompactMode() {
		return 1
	}
	return 2 // footer + hints
}
