package theme

import "testing"

func TestGrokNightHasRequiredFields(t *testing.T) {
	tok := GrokNight()
	if tok.BgBase == "" || tok.AccentUser == "" || tok.AccentAssistant == "" {
		t.Fatal("missing tokens")
	}
}

func TestSetAndCurrent(t *testing.T) {
	Set(Light())
	c := Current()
	if c.BgBase != Light().BgBase {
		t.Fatal("Set/Current mismatch")
	}
	Set(GrokNight())
}

func TestSetByNameAndCycle(t *testing.T) {
	if !SetByName("tokyonight") {
		t.Fatal("tokyonight")
	}
	if Name() != "tokyonight" {
		t.Fatal(Name())
	}
	_ = Cycle()
	SetByName("groknight")
}

func TestMotionFlag(t *testing.T) {
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
	// should never panic
	_ = FileIcon("main.go")
	_ = FileIcon("README.md")
	_ = ToolIcon("read_file")
	_ = GitStatusGlyph("M")
}
