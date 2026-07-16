package themepicker

import (
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/theme"
)

func TestPickerOpenPreviewConfirm(t *testing.T) {
	theme.Set(theme.GrokNight())
	var m Model
	m.Width = 60
	m.Open()
	if !m.Active {
		t.Fatal("active")
	}
	view := m.View()
	if !strings.Contains(view, "Theme") {
		t.Fatalf("view: %s", view)
	}
	// move to a named theme if possible
	for i := 0; i < len(m.Options); i++ {
		if m.Options[i].Name == "aurora" {
			m.Cursor = i
			m.previewCurrent()
			break
		}
	}
	m.Confirm()
	if !m.Done || !m.Confirmed {
		t.Fatal("confirm")
	}
}

func TestPickerCancelReverts(t *testing.T) {
	theme.Set(theme.GrokNight())
	var m Model
	m.Open()
	// preview something else
	theme.Set(theme.Aurora())
	m.Cancel()
	if theme.DisplayName() != "groknight" {
		t.Fatalf("revert got %s", theme.DisplayName())
	}
}
