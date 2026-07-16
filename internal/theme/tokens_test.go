package theme

import "testing"

func TestAuroraHasRequiredFields(t *testing.T) {
	tok := Aurora()
	if tok.BgBase == "" || tok.AccentAI == "" {
		t.Fatal("missing tokens")
	}
}

func TestSetAndCurrent(t *testing.T) {
	Set(Light())
	c := Current()
	if c.BgBase != Light().BgBase {
		t.Fatal("Set/Current mismatch")
	}
	Set(Aurora())
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
