// Package settings is a simple Grok-style /settings overlay.
package settings

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// Row is one settings line.
type Row struct {
	Key   string
	Value string
	Hint  string
}

// Model is the settings panel.
type Model struct {
	Active bool
	Rows   []Row
	Cursor int
	Width  int
	// Done when closed
	Done bool
	// Action when Enter on a row: "toggle:vim", etc.
	Action string
}

func New() Model { return Model{} }

// Open with current values.
func (m *Model) Open(rows []Row) {
	m.Active = true
	m.Done = false
	m.Action = ""
	m.Rows = rows
	m.Cursor = 0
}

func (m *Model) Close() {
	m.Active = false
	m.Done = true
	m.Action = ""
}

func (m *Model) Move(d int) {
	if len(m.Rows) == 0 {
		return
	}
	m.Cursor += d
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Rows) {
		m.Cursor = len(m.Rows) - 1
	}
}

func (m *Model) Activate() {
	if m.Cursor >= 0 && m.Cursor < len(m.Rows) {
		m.Action = m.Rows[m.Cursor].Key
	}
}

func (m Model) View() string {
	if !m.Active {
		return ""
	}
	t := theme.Current()
	w := m.Width
	if w > 72 {
		w = 72
	}
	if w < 40 {
		w = 40
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(t.BorderGlow).Render("⚙ Settings")
	sub := lipgloss.NewStyle().Foreground(t.TextMuted).Render("↑↓  Enter toggle/action  Esc close")
	var rows []string
	for i, r := range m.Rows {
		line := fmt.Sprintf("%-18s  %s", r.Key, r.Value)
		if r.Hint != "" {
			line += "  " + lipgloss.NewStyle().Foreground(t.TextMuted).Render(r.Hint)
		}
		if i == m.Cursor {
			line = lipgloss.NewStyle().Background(t.BgElevated).Foreground(t.AccentFocus).Bold(true).
				Width(w - 6).Render("› " + line)
		} else {
			line = lipgloss.NewStyle().Foreground(t.TextSecondary).Width(w - 6).Render("  " + line)
		}
		rows = append(rows, line)
	}
	body := title + "\n" + sub + "\n" +
		lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", w-6)) + "\n" +
		strings.Join(rows, "\n")
	return theme.OverlayStyle(w).Render(body)
}
