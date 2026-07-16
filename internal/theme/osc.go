package theme

import (
	"fmt"
	"os"
	"strings"
)

// OSC 12 sets cursor color to accent_user; OSC 112 resets on exit.
// Works in most modern terminals (not all SSH multiplexers).

// ApplyCursorColor sends OSC 12 with the current theme's AccentUser.
// No-op in minimal mode or when NO_COLOR / CODEFORGE_NO_OSC is set.
func ApplyCursorColor() {
	if MinimalMode() {
		return
	}
	if os.Getenv("NO_COLOR") != "" {
		return
	}
	if v := os.Getenv("CODEFORGE_NO_OSC"); v == "1" || strings.EqualFold(v, "true") {
		return
	}
	c := string(Current().AccentUser)
	if c == "" {
		return
	}
	// Convert lipgloss color to #RRGGBB for OSC
	hex := colorToHex(c)
	if hex == "" {
		return
	}
	// OSC 12 ; color ST  (BEL terminator also widely accepted)
	fmt.Fprintf(os.Stdout, "\x1b]12;%s\x07", hex)
}

// ResetCursorColor sends OSC 112 to restore the terminal default cursor color.
func ResetCursorColor() {
	if os.Getenv("CODEFORGE_NO_OSC") == "1" {
		return
	}
	fmt.Fprint(os.Stdout, "\x1b]112\x07")
}

// colorToHex converts #RRGGBB or ANSI index to a hex color for OSC.
func colorToHex(c string) string {
	c = strings.TrimSpace(c)
	if strings.HasPrefix(c, "#") && (len(c) == 7 || len(c) == 4) {
		return c
	}
	// ANSI 16 index → approximate hex
	if len(c) <= 3 {
		// try parse as int
		var n int
		if _, err := fmt.Sscanf(c, "%d", &n); err == nil && n >= 0 && n < len(ansi16Hex) {
			return ansi16Hex[n]
		}
	}
	return ""
}
