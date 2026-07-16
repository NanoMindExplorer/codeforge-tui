package onboarding

import (
	"strings"
	"testing"
)

func TestBrandHeader(t *testing.T) {
	h := BrandHeader()
	if !strings.Contains(h, "CodeForge") {
		t.Fatal(h)
	}
	if !strings.Contains(h, "By NanoMindExplorer") {
		t.Fatal(h)
	}
	// byline appears after title
	iTitle := strings.Index(h, "CodeForge")
	iBy := strings.Index(h, "By NanoMindExplorer")
	if iTitle < 0 || iBy < iTitle {
		t.Fatal("byline should be below title")
	}
}

func TestBrandHeaderPlain(t *testing.T) {
	p := BrandHeaderPlain()
	if p != "CodeForge\nBy NanoMindExplorer" {
		t.Fatal(p)
	}
}

func TestWelcomeStartsWithBrand(t *testing.T) {
	w := WelcomeMessage(nil, "gemini", "flash", true)
	if !strings.HasPrefix(w, "CodeForge\nBy NanoMindExplorer\n") {
		t.Fatal(w[:min(80, len(w))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
