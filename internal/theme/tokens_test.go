package theme

import (
	"os"
	"testing"
)

func TestGrokNightHasRequiredFields(t *testing.T) {
	tok := GrokNight()
	if tok.BgBase == "" || tok.AccentUser == "" || tok.AccentAssistant == "" {
		t.Fatal("missing tokens")
	}
	if tok.AccentThinking == "" || tok.ScrollbarFg == "" || tok.SelectionBorder == "" {
		t.Fatal("missing Phase 3 slots")
	}
	if tok.MdHeading == "" || tok.MdCode == "" {
		t.Fatal("missing md_* slots")
	}
}

func TestAllBuiltIns(t *testing.T) {
	for _, opt := range BuiltInThemes() {
		tok := opt.Factory()
		if tok.Name == "" {
			t.Fatalf("%s empty name", opt.Name)
		}
		if tok.AccentUser == "" {
			t.Fatalf("%s missing AccentUser", opt.Name)
		}
	}
}

func TestSetAndCurrent(t *testing.T) {
	Set(Light())
	c := Current()
	if c.Name != "grokday" && c.BgBase != Light().BgBase {
		// after quantize name should still be grokday
		if DisplayName() != "grokday" {
			t.Fatalf("expected grokday, got %s", DisplayName())
		}
	}
	Set(GrokNight())
}

func TestSetByNameAndCycle(t *testing.T) {
	if !SetByName("tokyonight") {
		t.Fatal("tokyonight")
	}
	if DisplayName() != "tokyonight" {
		t.Fatal(DisplayName())
	}
	if !SetByName("rosepine") {
		t.Fatal("rosepine")
	}
	if !SetByName("oscura") {
		t.Fatal("oscura")
	}
	_ = Cycle()
	SetByName("groknight")
}

func TestAutoMode(t *testing.T) {
	os.Setenv("CODEFORGE_APPEARANCE", "dark")
	defer os.Unsetenv("CODEFORGE_APPEARANCE")
	SetAutoMapping("groknight", "grokday")
	EnableAuto()
	if !IsAuto() {
		t.Fatal("expected auto")
	}
	if DisplayName() != "groknight" {
		t.Fatalf("dark auto → groknight, got %s", DisplayName())
	}
	os.Setenv("CODEFORGE_APPEARANCE", "light")
	ResolveAuto()
	if DisplayName() != "grokday" {
		t.Fatalf("light auto → grokday, got %s", DisplayName())
	}
	SetByName("aurora")
	if IsAuto() {
		t.Fatal("SetByName should clear auto")
	}
}

func TestMinimalMode(t *testing.T) {
	SetMinimal(true)
	if !MinimalMode() {
		t.Fatal("minimal")
	}
	if DisplayName() != "minimal" {
		t.Fatal(DisplayName())
	}
	if MotionEnabled() {
		t.Fatal("motion should be off in minimal")
	}
	SetMinimal(false)
	SetByName("groknight")
}

func TestQuantizeTruecolorPassthrough(t *testing.T) {
	os.Setenv("CODEFORGE_COLOR", "true")
	ResetColorLevelCache()
	defer func() {
		os.Unsetenv("CODEFORGE_COLOR")
		ResetColorLevelCache()
	}()
	tok := GrokNight()
	q := QuantizeTokens(tok)
	if string(q.AccentUser) != string(tok.AccentUser) {
		t.Fatalf("truecolor should pass through: %s vs %s", q.AccentUser, tok.AccentUser)
	}
}

func TestQuantize16(t *testing.T) {
	os.Setenv("CODEFORGE_COLOR", "16")
	ResetColorLevelCache()
	defer func() {
		os.Unsetenv("CODEFORGE_COLOR")
		ResetColorLevelCache()
	}()
	tok := QuantizeTokens(GrokNight())
	// AccentUser should become ANSI index
	s := string(tok.AccentUser)
	if s == "" || s[0] == '#' {
		t.Fatalf("expected ANSI index, got %q", s)
	}
}

func TestLayoutCompactMinimal(t *testing.T) {
	SetMinimal(false)
	SetCompact(false)
	full := CurrentLayout()
	if full.OuterVPad != 1 {
		t.Fatal(full)
	}
	SetCompact(true)
	c := CurrentLayout()
	if c.OuterVPad != 0 || c.OuterHPadLeft != 1 {
		t.Fatal(c)
	}
	SetMinimal(true)
	min := CurrentLayout()
	if min.OuterHPadLeft != 0 {
		t.Fatal(min)
	}
	SetMinimal(false)
	SetCompact(false)
}

func TestMotionFlag(t *testing.T) {
	SetMinimal(false)
	SetMotion(false)
	if MotionEnabled() {
		t.Fatal("expected motion off")
	}
	SetMotion(true)
	if !MotionEnabled() {
		t.Fatal("expected motion on")
	}
}

func TestFileIconFallback(t *testing.T) {
	_ = FileIcon("main.go")
	_ = FileIcon("README.md")
	_ = ToolIcon("read_file")
	_ = GitStatusGlyph("M")
}

func TestGlamourStyleName(t *testing.T) {
	SetByName("groknight")
	if GlamourStyleName() != "dark" {
		t.Fatal(GlamourStyleName())
	}
	SetByName("grokday")
	if GlamourStyleName() != "light" {
		t.Fatal(GlamourStyleName())
	}
}

func TestThemeNamesForPicker(t *testing.T) {
	os.Setenv("CODEFORGE_COLOR", "16")
	ResetColorLevelCache()
	defer func() {
		os.Unsetenv("CODEFORGE_COLOR")
		ResetColorLevelCache()
	}()
	opts := ThemeNamesForPicker()
	for _, o := range opts {
		if o.Truecolor {
			t.Fatalf("truecolor theme leaked on 16-color: %s", o.Name)
		}
	}
}
