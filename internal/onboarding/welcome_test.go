package onboarding

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStatusCardGoldenHealthy(t *testing.T) {
	// Isolate env keys
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "GEMINI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	card := StatusCard(nil, "gemini", "gemini-2.5-flash", true)
	// golden structure (Q5.1)
	for _, want := range []string{
		BrandByline,
		"Status  ✓  gemini",
		"gemini-2.5-flash",
		"Shift+Tab",
		"/help",
		"/ ___|___", // ASCII art fragment (FIGlet "CodeForge")
	} {
		if !strings.Contains(card, want) {
			t.Fatalf("missing %q in:\n%s", want, card)
		}
	}
	// single status panel: should not flood with multi-block "Quick start" walls when healthy
	if strings.Count(card, "Quick start") > 0 {
		t.Fatal("healthy card should not include no-key quick start")
	}
}

func TestStatusCardUnhealthy(t *testing.T) {
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "GEMINI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	card := StatusCard(nil, "", "", false)
	if !strings.Contains(card, "No API key") {
		t.Fatal(card)
	}
	if !strings.Contains(card, "/setup") {
		t.Fatal(card)
	}
}

func TestEmptyStateCopy(t *testing.T) {
	nk := EmptyStateNoKey()
	if !strings.Contains(nk, "/setup") || !strings.Contains(nk, "GEMINI") {
		t.Fatal(nk)
	}
	np := EmptyStateNoProject("/tmp/empty-proj")
	if !strings.Contains(np, "empty-proj") || !strings.Contains(np, "@") {
		t.Fatal(np)
	}
}

func TestProjectLooksEmpty(t *testing.T) {
	empty := t.TempDir()
	if !ProjectLooksEmpty(empty) {
		t.Fatal("empty dir should look empty")
	}
	// one file still "empty-ish"
	_ = os.WriteFile(filepath.Join(empty, "README.md"), []byte("# hi"), 0o644)
	if !ProjectLooksEmpty(empty) {
		t.Fatal("single md still empty-ish")
	}
	// src/ makes it a project
	full := t.TempDir()
	_ = os.Mkdir(filepath.Join(full, "src"), 0o755)
	if ProjectLooksEmpty(full) {
		t.Fatal("src/ means not empty")
	}
	// two code files
	two := t.TempDir()
	_ = os.WriteFile(filepath.Join(two, "a.go"), []byte("package a"), 0o644)
	_ = os.WriteFile(filepath.Join(two, "b.go"), []byte("package b"), 0o644)
	if ProjectLooksEmpty(two) {
		t.Fatal("two go files")
	}
}

func TestWelcomeMessageAlias(t *testing.T) {
	// WelcomeMessage == StatusCard
	a := WelcomeMessage(nil, "grok", "grok-4.5", true)
	b := StatusCard(nil, "grok", "grok-4.5", true)
	if a != b {
		t.Fatal("WelcomeMessage should equal StatusCard")
	}
}
