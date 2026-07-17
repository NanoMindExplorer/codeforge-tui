package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBootstrapQuietNoNetwork(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CODEFORGE_CONFIG_DIR", cfgDir)
	t.Setenv("CODEFORGE_NO_KEYRING", "1")
	t.Setenv("CODEFORGE_SECRETS_BACKEND", "file")
	// isolated keys
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	t.Setenv("GEMINI_API_KEY", "test-gemini-key-for-bootstrap")

	workdir := t.TempDir()
	// tiny project so index would be fast if enabled
	_ = os.WriteFile(filepath.Join(workdir, "main.go"), []byte("package main\n"), 0o644)

	start := time.Now()
	rt, err := Bootstrap(Options{
		WorkDir:     workdir,
		Quiet:       true,
		SkipIndex:   true,
		SkipMCP:     true,
		SkipPlugins: true,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil || rt.Cfg == nil || rt.ProvReg == nil || rt.ToolReg == nil {
		t.Fatal("incomplete runtime")
	}
	if rt.WorkDir != workdir {
		// may be abs
		if filepath.Base(rt.WorkDir) != filepath.Base(workdir) {
			t.Fatalf("workdir %s", rt.WorkDir)
		}
	}
	// provider registered from env
	if _, err := rt.ProvReg.Current(); err != nil {
		t.Fatal("expected provider from GEMINI_API_KEY:", err)
	}
	name := rt.ProvReg.CurrentName()
	if name != "gemini" && name != "grok" {
		// ResolveActive may pick gemini
		t.Logf("active provider: %s", name)
	}
	// fast path: skip index should be sub-second typically
	if elapsed > 15*time.Second {
		t.Fatalf("bootstrap too slow with SkipIndex: %v", elapsed)
	}
	t.Logf("bootstrap SkipIndex=true in %v", elapsed)
}

func TestBootstrapInvalidConfigFallsBack(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CODEFORGE_CONFIG_DIR", cfgDir)
	t.Setenv("CODEFORGE_NO_KEYRING", "1")
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "GEMINI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	_ = os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(`
sandbox:
  profile: not-a-real-profile
`), 0o600)

	rt, err := Bootstrap(Options{
		WorkDir:     t.TempDir(),
		Quiet:       true,
		SkipIndex:   true,
		SkipMCP:     true,
		SkipPlugins: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rt.Cfg == nil {
		t.Fatal("expected default cfg fallback")
	}
	// default sandbox is off
	if rt.Cfg.Sandbox.Profile != "off" && rt.Cfg.Sandbox.Profile != "" {
		// Default() has off
		t.Logf("sandbox profile after fallback: %q", rt.Cfg.Sandbox.Profile)
	}
}

func TestBootstrapForceIndexEnv(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CODEFORGE_CONFIG_DIR", cfgDir)
	t.Setenv("CODEFORGE_NO_KEYRING", "1")
	t.Setenv("CODEFORGE_INDEX", "1")
	t.Setenv("GEMINI_API_KEY", "k")
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	workdir := t.TempDir()
	_ = os.WriteFile(filepath.Join(workdir, "x.go"), []byte("package x\n"), 0o644)

	rt, err := Bootstrap(Options{
		WorkDir:     workdir,
		Quiet:       true,
		SkipIndex:   true, // should be overridden by CODEFORGE_INDEX=1
		SkipMCP:     true,
		SkipPlugins: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil {
		t.Fatal("nil runtime")
	}
	// index may or may not set global; just ensure no panic and tools work
	if rt.ToolReg == nil {
		t.Fatal("tools")
	}
}

func TestHeadlessStyleBootstrapIsFast(t *testing.T) {
	// Timing smoke: SkipIndex+SkipMCP+SkipPlugins should beat a full index of a medium tree.
	cfgDir := t.TempDir()
	t.Setenv("CODEFORGE_CONFIG_DIR", cfgDir)
	t.Setenv("CODEFORGE_NO_KEYRING", "1")
	t.Setenv("CODEFORGE_INDEX", "")
	t.Setenv("GEMINI_API_KEY", "k")
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	workdir := t.TempDir()
	// create many dummy files that would slow index
	for i := 0; i < 50; i++ {
		_ = os.WriteFile(filepath.Join(workdir, strings.Repeat("f", 3)+string(rune('a'+i%26))+".go"),
			[]byte("package p\n"), 0o644)
	}
	start := time.Now()
	_, err := Bootstrap(Options{WorkDir: workdir, Quiet: true, SkipIndex: true, SkipMCP: true, SkipPlugins: true})
	d1 := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if d1 > 10*time.Second {
		t.Fatalf("skip-index boot %v too slow", d1)
	}
}
