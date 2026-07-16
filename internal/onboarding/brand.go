package onboarding

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// BrandName is the product title shown at onboarding start.
const BrandName = "CodeForge"

// BrandByline is the small credit under the title.
const BrandByline = "By NanoMindExplorer"

// BrandHeader returns the boxed start-of-onboarding title block (plain text).
//
//	      CodeForge
//	By NanoMindExplorer
func BrandHeader() string {
	var b strings.Builder
	WriteBrandStart(&b, false)
	return b.String()
}

// BrandHeaderPlain is title + small byline without a box (for TUI system messages).
func BrandHeaderPlain() string {
	return BrandName + "\n" + BrandByline
}

// WriteBrandStart prints the onboarding start branding to out.
// When useANSI is true (and NO_COLOR unset), the byline is dimmed so it reads smaller.
func WriteBrandStart(out io.Writer, useANSI bool) {
	if out == nil {
		return
	}
	const width = 42
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  ╔"+strings.Repeat("═", width)+"╗")
	fmt.Fprintln(out, "  ║"+strings.Repeat(" ", width)+"║")
	// Prominent product name (reads large)
	fmt.Fprintln(out, "  ║"+padCenter(BrandName, width)+"║")
	fmt.Fprintln(out, "  ║"+strings.Repeat(" ", width)+"║")
	// Smaller credit line
	if useANSI && os.Getenv("NO_COLOR") == "" && isANSITerm() {
		left := (width - len(BrandByline)) / 2
		right := width - len(BrandByline) - left
		// dim byline
		fmt.Fprintln(out, "  ║"+strings.Repeat(" ", left)+"\033[2m"+BrandByline+"\033[0m"+strings.Repeat(" ", right)+"║")
	} else {
		fmt.Fprintln(out, "  ║"+padCenter(BrandByline, width)+"║")
	}
	fmt.Fprintln(out, "  ║"+strings.Repeat(" ", width)+"║")
	fmt.Fprintln(out, "  ╚"+strings.Repeat("═", width)+"╝")
	fmt.Fprintln(out)
}

func padCenter(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	left := (width - len(s)) / 2
	right := width - len(s) - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}

func isANSITerm() bool {
	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}
