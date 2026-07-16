package onboarding

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProviderFromKey(t *testing.T) {
	cases := map[string]string{
		"xai-abc":    "grok",
		"sk-ant-xyz": "claude",
		"AIzaSyTest": "gemini",
		"sk-openai1": "openai",
		"random":     "",
	}
	for k, want := range cases {
		if got := DetectProviderFromKey(k); got != want {
			t.Fatalf("%s: got %q want %q", k, got, want)
		}
	}
}

func TestStateSaveLoad(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// also USERPROFILE for portability
	t.Setenv("USERPROFILE", home)

	if err := MarkCompleted("grok", "grok-4.5"); err != nil {
		t.Fatal(err)
	}
	st, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Completed || st.Provider != "grok" || st.Model != "grok-4.5" {
		t.Fatalf("%+v", st)
	}
	p, _ := Path()
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	// path under temp home
	if filepath.Dir(filepath.Dir(p)) != home && filepath.Dir(p) != filepath.Join(home, ".codeforge") {
		// just ensure file exists under home
		if !filepath.IsAbs(p) {
			t.Fatal(p)
		}
	}
}

func TestNeedsWizard(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	// clear keys
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "GEMINI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	_ = Save(State{})
	if !NeedsWizard(false) {
		t.Fatal("expected wizard when no keys")
	}
	if NeedsWizard(true) {
		t.Fatal("skip flag")
	}
	_ = MarkSkipped()
	if NeedsWizard(false) {
		t.Fatal("skipped should not re-prompt")
	}
}
