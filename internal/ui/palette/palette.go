// Package palette implements a fuzzy Ctrl+K command palette.
package palette

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
	"github.com/sahilm/fuzzy"
)

// Item is one selectable palette entry.
type Item struct {
	ID          string // e.g. "cmd:act", "file:main.go", "session:..."
	Label       string
	Description string
	Category    string // command | file | session
}

// Model is the fuzzy overlay state.
type Model struct {
	Active  bool
	Query   string
	Items   []Item
	Filtered []Item
	Cursor  int
	Width   int
	Height  int
	// Done + Selected set when user confirms
	Done     bool
	Selected *Item
}

func New() Model {
	return Model{}
}

func (m *Model) SetSize(w, h int) {
	m.Width = w
	m.Height = h
}

func (m *Model) Open(items []Item) {
	m.Active = true
	m.Query = ""
	m.Items = items
	m.Filtered = items
	m.Cursor = 0
	m.Done = false
	m.Selected = nil
}

func (m *Model) Close() {
	m.Active = false
	m.Done = false
	m.Selected = nil
	m.Query = ""
}

func (m *Model) SetQuery(q string) {
	m.Query = q
	m.refilter()
}

func (m *Model) Type(s string) {
	m.Query += s
	m.refilter()
}

func (m *Model) Backspace() {
	if len(m.Query) == 0 {
		return
	}
	// UTF-8 safe
	r := []rune(m.Query)
	m.Query = string(r[:len(r)-1])
	m.refilter()
}

func (m *Model) refilter() {
	if strings.TrimSpace(m.Query) == "" {
		m.Filtered = m.Items
		m.Cursor = 0
		return
	}
	labels := make([]string, len(m.Items))
	for i, it := range m.Items {
		labels[i] = it.Label + " " + it.Description + " " + it.Category
	}
	matches := fuzzy.Find(m.Query, labels)
	m.Filtered = make([]Item, 0, len(matches))
	for _, match := range matches {
		m.Filtered = append(m.Filtered, m.Items[match.Index])
	}
	m.Cursor = 0
}

func (m *Model) Move(delta int) {
	if len(m.Filtered) == 0 {
		return
	}
	m.Cursor += delta
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Filtered) {
		m.Cursor = len(m.Filtered) - 1
	}
}

func (m *Model) Confirm() {
	m.Done = true
	m.Active = false
	if len(m.Filtered) > 0 && m.Cursor >= 0 && m.Cursor < len(m.Filtered) {
		sel := m.Filtered[m.Cursor]
		m.Selected = &sel
	}
}

func (m *Model) Cancel() {
	m.Done = true
	m.Active = false
	m.Selected = nil
}

// View renders the modal overlay content (caller composites over main UI).
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
		w = m.Width
		if w < 30 {
			w = 30
		}
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(t.BorderGlow).Render("⌘ Command Palette")
	input := lipgloss.NewStyle().Foreground(t.AccentAI).Render("> "+m.Query+"▌")

	maxShow := 10
	if m.Height > 0 && m.Height/3 < maxShow {
		maxShow = m.Height / 3
	}
	if maxShow < 5 {
		maxShow = 5
	}

	var rows []string
	for i, it := range m.Filtered {
		if i >= maxShow {
			rows = append(rows, lipgloss.NewStyle().Foreground(t.TextMuted).Render(
				fmt.Sprintf("  … +%d more", len(m.Filtered)-maxShow)))
			break
		}
		cat := lipgloss.NewStyle().Foreground(t.TextMuted).Render(fmt.Sprintf("[%s]", it.Category))
		line := fmt.Sprintf("%s %s  %s", it.Label, cat, it.Description)
		if i == m.Cursor {
			line = lipgloss.NewStyle().
				Background(t.BgElevated).
				Foreground(t.AccentFocus).
				Bold(true).
				Width(w - 6).
				Render("› " + line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(t.TextSecondary).
				Width(w - 6).
				Render("  " + line)
		}
		rows = append(rows, line)
	}
	if len(rows) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(t.TextMuted).Render("  (no matches)"))
	}

	body := title + "\n" + input + "\n" +
		lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", w-6)) + "\n" +
		strings.Join(rows, "\n") + "\n" +
		lipgloss.NewStyle().Foreground(t.TextMuted).Italic(true).Render("↑↓ navigate  Enter select  Esc close")

	return theme.OverlayStyle(w).Render(body)
}
