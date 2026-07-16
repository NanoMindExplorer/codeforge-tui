package doctor

import (
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/provider"
)

func TestRunEmptyRegistry(t *testing.T) {
	reg := provider.NewRegistry()
	r := Run(Options{Registry: reg, Version: "1.9.0", WorkDir: t.TempDir()})
	if r.OK {
		t.Fatal("expected issues with empty registry")
	}
	s := r.String()
	if !strings.Contains(s, "doctor") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "Terminal") {
		t.Fatal(s)
	}
}

func TestRunWithGrok(t *testing.T) {
	reg := provider.NewRegistry()
	p := provider.NewGrokProvider("xai-test-key-not-real", "grok-4.5")
	_ = reg.Register(p)
	_ = reg.Switch("grok")
	r := Run(Options{Registry: reg, Version: "1.9.0"})
	if !strings.Contains(r.String(), "grok") {
		t.Fatal(r.String())
	}
}
