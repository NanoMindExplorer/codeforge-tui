package blocks

import (
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/theme"
)

func TestBlockFoldAndKinds(t *testing.T) {
	theme.Set(theme.GrokNight())
	s := NewStore()
	s.SetSize(80, 20)
	s.AddUser("hello world")
	s.StartAssistant()
	s.AppendAssistantChunk("Hi **there**")
	s.SealAssistant()
	s.AddToolCall("read_file", `{"path":"main.go"}`)
	s.AddToolResult("read_file", "package main\n", true)

	if s.Len() != 4 {
		t.Fatalf("blocks=%d\n%s", s.Len(), s.DebugString())
	}
	// collapse tool call
	s.selected = 2
	s.ToggleCollapse()
	if !s.Blocks()[2].Collapsed {
		t.Fatal("expected collapsed")
	}
	view := s.View()
	if !strings.Contains(view, "read_file") {
		t.Fatalf("view missing tool header:\n%s", view)
	}
	// expand all
	s.ExpandAll()
	if s.Blocks()[2].Collapsed {
		t.Fatal("expected expanded")
	}
}

func TestFollowTailAndManualScroll(t *testing.T) {
	theme.Set(theme.GrokNight())
	s := NewStore()
	s.SetSize(60, 8)
	for i := 0; i < 20; i++ {
		s.AddSystem("line system message number " + strings.Repeat("x", 20))
	}
	if !s.Following() {
		t.Fatal("should follow")
	}
	s.LineUp()
	if s.Following() {
		t.Fatal("manual scroll should break follow")
	}
	s.GotoBottom()
	if !s.Following() {
		t.Fatal("G should resume follow")
	}
}

func TestStickyHeader(t *testing.T) {
	theme.Set(theme.GrokNight())
	s := NewStore()
	s.SetSize(70, 6)
	s.AddUser("implement authentication for the API gateway carefully")
	for i := 0; i < 15; i++ {
		s.AddSystem("filler content " + strings.Repeat("z", 40))
	}
	s.follow = false
	s.offset = 10
	st := s.StickyUserTitle()
	if st == "" {
		t.Fatal("expected sticky user title")
	}
	if !strings.Contains(st, "authentication") && !strings.Contains(st, "implement") {
		t.Fatalf("sticky=%q", st)
	}
}

func TestStreamingAssistantBlock(t *testing.T) {
	s := NewStore()
	s.SetSize(80, 15)
	s.AddUser("hi")
	s.AppendAssistantChunk("Hello")
	s.AppendAssistantChunk(" world")
	if s.Len() != 2 {
		t.Fatal(s.Len())
	}
	s.SealAssistant()
	body := s.Blocks()[1].Body
	if body != "Hello world" {
		t.Fatalf("%q", body)
	}
	if s.Blocks()[1].Streaming {
		t.Fatal("should be sealed")
	}
}

func TestSelectNavigation(t *testing.T) {
	s := NewStore()
	s.SetSize(80, 20)
	s.AddUser("a")
	s.AddSystem("b")
	s.AddSystem("c")
	s.SelectPrev()
	s.SelectPrev()
	if s.SelectedIndex() < 0 {
		t.Fatal("no selection")
	}
	s.SelectNext()
	s.ToggleCollapse()
}

func TestDiffMeta(t *testing.T) {
	d := "--- a\n+++ b\n@@\n-line\n+line2\n+line3\n"
	m := DiffMeta(d)
	if m != "+2 -1" {
		t.Fatal(m)
	}
}

func TestThinkingSeal(t *testing.T) {
	s := NewStore()
	s.AddThinking("planning…")
	s.SealThinking()
	bl := s.Blocks()
	if len(bl) != 1 || bl[0].Streaming {
		t.Fatal(bl)
	}
}
