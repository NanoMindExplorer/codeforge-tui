package theme

// Layout padding mirrors Grok pager.toml scrollback.layout defaults.

// Layout holds outer/inner padding for chrome.
type Layout struct {
	OuterVPad     int // top/bottom viewport margin
	OuterHPadLeft int
	OuterHPadRight int
	BlockPadLeft  int
	BlockPadRight int
	PromptPadV    int // vertical padding inside prompt frame
}

// CurrentLayout returns padding matrix for full / compact / minimal.
func CurrentLayout() Layout {
	if MinimalMode() {
		return Layout{
			OuterVPad:      0,
			OuterHPadLeft:  0,
			OuterHPadRight: 0,
			BlockPadLeft:   1,
			BlockPadRight:  0,
			PromptPadV:     0,
		}
	}
	if CompactMode() {
		return Layout{
			OuterVPad:      0,
			OuterHPadLeft:  1,
			OuterHPadRight: 1,
			BlockPadLeft:   1,
			BlockPadRight:  1,
			PromptPadV:     0,
		}
	}
	// Grok defaults
	return Layout{
		OuterVPad:      1,
		OuterHPadLeft:  2,
		OuterHPadRight: 2,
		BlockPadLeft:   2,
		BlockPadRight:  2,
		PromptPadV:     1,
	}
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
