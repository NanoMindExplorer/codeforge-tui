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

// codeForgeASCII is FIGlet-style art for "CodeForge" (standard / slant-ish).
// Keep width reasonable for 80-col terminals.
const codeForgeASCII = `
   ____          _      _____
  / ___|___   __| | ___|  ___|__  _ __ __ _  ___
 | |   / _ \ / _` + "`" + ` |/ _ \ |_ / _ \| '__/ _` + "`" + ` |/ _ \
 | |__| (_) | (_| |  __/  _| (_) | | | (_| |  __/
  \____\___/ \__,_|\___|_|  \___/|_|  \__, |\___|
                                      |___/
`

// BrandHeader returns the full ASCII start block + byline (plain text, no ANSI).
func BrandHeader() string {
	var b strings.Builder
	WriteBrandStart(&b, false)
	return b.String()
}

// BrandHeaderPlain is a compact form for narrow UI (still ASCII title + byline).
func BrandHeaderPlain() string {
	return strings.TrimSpace(codeForgeASCII) + "\n\n  " + BrandByline
}

// BrandASCII returns just the CodeForge FIGlet lines (trimmed).
func BrandASCII() string {
	return strings.TrimRight(strings.TrimLeft(codeForgeASCII, "\n"), "\n")
}

// WriteBrandStart prints onboarding start branding:
// large ASCII "CodeForge", then a smaller "By NanoMindExplorer" byline.
func WriteBrandStart(out io.Writer, useANSI bool) {
	if out == nil {
		return
	}
	art := BrandASCII()
	lines := strings.Split(art, "\n")
	// measure art width for optional framing
	maxW := 0
	for _, ln := range lines {
		if len(ln) > maxW {
			maxW = len(ln)
		}
	}
	if maxW < len(BrandByline)+4 {
		maxW = len(BrandByline) + 4
	}

	fmt.Fprintln(out)
	// optional cyan title if ANSI
	colorOn, colorOff, dimOn, dimOff := "", "", "", ""
	if useANSI && os.Getenv("NO_COLOR") == "" && isANSITerm() {
		colorOn = "\033[1;36m" // bold cyan
		colorOff = "\033[0m"
		dimOn = "\033[2m" // dim byline (reads smaller)
		dimOff = "\033[0m"
	}

	for _, ln := range lines {
		fmt.Fprintln(out, "  "+colorOn+ln+colorOff)
	}
	fmt.Fprintln(out)
	// byline centered under art, dim / plain
	pad := 0
	if maxW > len(BrandByline) {
		pad = (maxW - len(BrandByline)) / 2
	}
	fmt.Fprintln(out, "  "+strings.Repeat(" ", pad)+dimOn+BrandByline+dimOff)
	fmt.Fprintln(out)
}

func isANSITerm() bool {
	term := os.Getenv("TERM")
	return term != "" && term != "dumb"
}
