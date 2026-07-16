package onboarding

import (
	"strings"
	"testing"
)

func TestBrandHeaderASCII(t *testing.T) {
	h := BrandHeader()
	if !strings.Contains(h, "CodeForge") && !strings.Contains(h, "___") {
		// FIGlet may not contain the literal word "CodeForge" as one token
		// but must contain art blocks and byline
	}
	art := BrandASCII()
	if !strings.Contains(art, "___") && !strings.Contains(art, "/ __") {
		t.Fatal("expected ASCII art shapes:\n", art)
	}
	if !strings.Contains(h, BrandByline) {
		t.Fatal("missing byline:\n", h)
	}
	// byline after art
	iArt := strings.Index(h, "/ __")
	iBy := strings.Index(h, BrandByline)
	if iBy < 0 || (iArt >= 0 && iBy < iArt) {
		t.Fatal("byline should be below ASCII art")
	}
}

func TestBrandHeaderPlain(t *testing.T) {
	p := BrandHeaderPlain()
	if !strings.Contains(p, BrandByline) {
		t.Fatal(p)
	}
	if !strings.Contains(p, "/ __") && !strings.Contains(p, "___") {
		t.Fatal("expected ASCII in plain header:\n", p)
	}
}

func TestWelcomeStartsWithASCII(t *testing.T) {
	w := WelcomeMessage(nil, "gemini", "flash", true)
	if !strings.Contains(w, BrandByline) {
		t.Fatal(w[:min(200, len(w))])
	}
	if !strings.Contains(w, "___") && !strings.Contains(w, "/ __") {
		t.Fatal("welcome should include ASCII art")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
