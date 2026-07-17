package settings

import (
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/theme"
)

func TestSettingsOpenMoveActivate(t *testing.T) {
	theme.Set(theme.Aurora())
	theme.SetMotion(false)
	m := New()
	m.Width = 60
	m.Open([]Row{
		{Key: "vim_mode", Value: "off", Hint: "/vim-mode"},
		{Key: "compact", Value: "off", Hint: "/compact-mode"},
		{Key: "sandbox", Value: "off", Hint: "/sandbox"},
	})
	if !m.Active || m.Cursor != 0 {
		t.Fatal("open")
	}
	m.Move(1)
	if m.Cursor != 1 {
		t.Fatal(m.Cursor)
	}
	m.Move(10)
	if m.Cursor != 2 {
		t.Fatal("clamp bottom")
	}
	m.Move(-100)
	if m.Cursor != 0 {
		t.Fatal("clamp top")
	}
	m.Activate()
	if m.Action != "vim_mode" {
		t.Fatal(m.Action)
	}
	view := m.View()
	if !strings.Contains(view, "Settings") {
		t.Fatal(view)
	}
	if !strings.Contains(view, "vim_mode") {
		t.Fatal(view)
	}
	m.Close()
	if m.Active || !m.Done {
		t.Fatal("close")
	}
	if m.View() != "" {
		t.Fatal("inactive view empty")
	}
}

func TestSettingsEmptyRows(t *testing.T) {
	m := New()
	m.Open(nil)
	m.Move(1)
	m.Activate()
	if m.Action != "" {
		t.Fatal(m.Action)
	}
}
