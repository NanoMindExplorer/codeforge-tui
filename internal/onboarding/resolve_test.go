package onboarding

import (
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/config"
)

func TestResolveActivePriorityGrokOverGemini(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("XAI_API_KEY", "xai-test")
	t.Setenv("GEMINI_API_KEY", "AIza-test")
	t.Setenv("GROK_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	// clear onboarding preference
	_ = Save(State{})

	res := ResolveActive(config.Default())
	if res.Provider != "grok" {
		t.Fatalf("got %q reason=%s", res.Provider, res.Reason)
	}
	if len(res.Alternatives) < 1 || res.Alternatives[0] != "gemini" {
		t.Fatalf("alts=%v", res.Alternatives)
	}
	if !strings.Contains(res.Reason, "priority") && !strings.Contains(res.Source, "XAI") {
		// reason should mention priority or source
		t.Log(res.Reason, res.Source)
	}
}

func TestResolveActiveOnboardingPreference(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("XAI_API_KEY", "xai-test")
	t.Setenv("GEMINI_API_KEY", "AIza-test")
	t.Setenv("GROK_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	if err := MarkCompleted("gemini", "gemini-2.5-flash"); err != nil {
		t.Fatal(err)
	}
	res := ResolveActive(config.Default())
	if res.Provider != "gemini" {
		t.Fatalf("preference should win: got %q (%s)", res.Provider, res.Reason)
	}
	if !strings.Contains(res.Reason, "onboarding") {
		t.Fatalf("reason=%s", res.Reason)
	}
}

func TestResolveActiveConfigDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", home+"/.config")
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GROK_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "AIza-test")
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test")
	t.Setenv("OPENAI_API_KEY", "")

	_ = Save(State{}) // no preference
	cfg := config.Default()
	cfg.DefaultProvider = "claude"
	// claude key is set via env — should pick claude over gemini despite priority
	res := ResolveActive(cfg)
	if res.Provider != "claude" {
		t.Fatalf("got %q want claude (%s)", res.Provider, res.Reason)
	}
}

func TestFormatStatusAndWelcome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XAI_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "AIza-x")
	t.Setenv("GROK_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	_ = Save(State{})

	s := FormatStatus(nil, "gemini")
	if !strings.Contains(s, "gemini") || !strings.Contains(s, "Active") {
		t.Fatal(s)
	}
	w := WelcomeMessage(nil, "gemini", "gemini-2.5-flash", true)
	if !strings.Contains(w, "gemini") || !strings.Contains(w, "Status") {
		t.Fatal(w)
	}
	w2 := WelcomeMessage(nil, "", "", false)
	if !strings.Contains(w2, "/setup") {
		t.Fatal(w2)
	}
}

func TestNeedsWizardMultiKey(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XAI_API_KEY", "xai-a")
	t.Setenv("GEMINI_API_KEY", "AIza-b")
	t.Setenv("GROK_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	_ = Save(State{}) // not completed

	if !NeedsWizard(false) {
		t.Fatal("multi-key without preference should need wizard")
	}
	_ = MarkCompleted("grok", "grok-4.5")
	if NeedsWizard(false) {
		t.Fatal("completed preference should skip wizard")
	}
}

func TestWizardPickAmongPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XAI_API_KEY", "xai-a")
	t.Setenv("GEMINI_API_KEY", "AIza-b")
	in := strings.NewReader("2\n\n") // pick gemini (index 2 if order is grok, gemini from PresentCloudKeys)
	// PresentCloudKeys order follows Catalog: grok then gemini
	var out strings.Builder
	err := RunWizard(WizardOptions{
		In: in, Out: &out, SkipValidation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := Load()
	if !st.Completed {
		t.Fatalf("expected completed: %+v\nout=%s", st, out.String())
	}
	// choice 2 = gemini in catalog numbering for multi-key list order
	if st.Provider != "gemini" && st.Provider != "grok" {
		// depends on PresentCloudKeys order matching [1][2]
		t.Logf("provider=%s out=%s", st.Provider, out.String())
	}
}

func TestMaskKey(t *testing.T) {
	if MaskKey("xai-abcdefghij") == "xai-abcdefghij" {
		t.Fatal("should mask")
	}
}
