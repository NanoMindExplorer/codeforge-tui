package pager

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	d := Defaults()
	if !d.ScrollbarEnabled() || !d.StickyHeaders() {
		t.Fatal("defaults")
	}
	if d.ToolBulletChar() != "◆" {
		t.Fatal(d.ToolBulletChar())
	}
	if d.AnimationFPS() != 30 {
		t.Fatal(d.AnimationFPS())
	}
}

func TestParseTOMLGrokShape(t *testing.T) {
	raw := `
[scrollback.layout]
outer_vpad = 0
outer_hpad_left = 1
block_pad_left = 1

[scrollback.scrollbar]
enabled = false

[scrollback.display]
sticky_headers = false
expandable_indicator_char = ">"

[scrollback.blocks.tool]
bullet = "triangle"

[scrollback.blocks.thinking]
truncate_lines = 5
header = false

[animation]
fps = 15

[ui]
scroll_speed = 80
invert_scroll = true
show_thinking_blocks = true
max_thoughts_width = 80
`
	c, err := parseTOML(raw)
	if err != nil {
		t.Fatal(err)
	}
	// merge over defaults to fill nils
	full := merge(Defaults(), c)
	if full.Layout.OuterVPad == nil || *full.Layout.OuterVPad != 0 {
		t.Fatalf("vpad %+v", full.Layout.OuterVPad)
	}
	if full.ScrollbarEnabled() {
		t.Fatal("scrollbar should be off")
	}
	if full.StickyHeaders() {
		t.Fatal("sticky off")
	}
	if full.ToolBulletChar() != "▶" {
		t.Fatal(full.ToolBulletChar())
	}
	if full.ThinkingTruncateLines() != 5 {
		t.Fatal(full.ThinkingTruncateLines())
	}
	if full.ThinkingHeader() {
		t.Fatal("header off")
	}
	if full.AnimationFPS() != 15 {
		t.Fatal(full.AnimationFPS())
	}
	if !full.InvertScroll() {
		t.Fatal("invert")
	}
	if full.MaxThoughtsWidth() != 80 {
		t.Fatal(full.MaxThoughtsWidth())
	}
}

func TestLoadFromDir(t *testing.T) {
	dir := t.TempDir()
	pd := filepath.Join(dir, ".codeforge")
	_ = os.MkdirAll(pd, 0755)
	_ = os.WriteFile(filepath.Join(pd, "pager.toml"), []byte(`
[scrollback.layout]
outer_vpad = 0
[scrollback.blocks.tool]
bullet = "dot"
`), 0644)
	c := Load(dir)
	if c.Source == "" {
		t.Fatal("no source")
	}
	if c.ToolBulletChar() != "·" {
		t.Fatal(c.ToolBulletChar())
	}
	if !c.ScrollbarEnabled() {
		t.Fatal("scrollbar default true")
	}
}

func TestYAMLLoad(t *testing.T) {
	dir := t.TempDir()
	pd := filepath.Join(dir, ".codeforge")
	_ = os.MkdirAll(pd, 0755)
	_ = os.WriteFile(filepath.Join(pd, "pager.yaml"), []byte(`
layout:
  outer_vpad: 2
blocks:
  tool:
    bullet: circle
`), 0644)
	c := Load(dir)
	if c.Layout.OuterVPad == nil || *c.Layout.OuterVPad != 2 {
		t.Fatal(c.Layout)
	}
	if c.ToolBulletChar() != "●" {
		t.Fatal(c.ToolBulletChar())
	}
}

func TestScrollSpeedMult(t *testing.T) {
	c := Defaults()
	if c.ScrollSpeedMult() < 0.9 || c.ScrollSpeedMult() > 1.1 {
		t.Fatal(c.ScrollSpeedMult())
	}
	n := 100
	c.UI.ScrollSpeed = &n
	if c.ScrollSpeedMult() < 5 {
		t.Fatal(c.ScrollSpeedMult())
	}
}
