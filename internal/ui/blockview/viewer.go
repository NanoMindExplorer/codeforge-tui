// Package blockview is the fullscreen block viewer (Phase 7 Enter).
package blockview

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
	"github.com/muesli/reflow/wordwrap"
)

// Content is a block snapshot for viewing/copying.
type Content struct {
	ID    string
	Title string
	Body  string
	Meta  string
}

// Model shows one block full-screen.
type Model struct {
	Active bool
	Block  Content
	Offset int
	Width  int
	Height int
	Done   bool
}

func New() Model { return Model{} }

func (m *Model) Open(c Content) {
	m.Active = true
	m.Done = false
	m.Block = c
	m.Offset = 0
}

func (m *Model) Close() {
	m.Active = false
	m.Done = true
}

func (m *Model) Scroll(d int) {
	lines := m.lines()
	vh := m.viewportH()
	m.Offset += d
	maxOff := len(lines) - vh
	if maxOff < 0 {
		maxOff = 0
	}
	if m.Offset < 0 {
		m.Offset = 0
	}
	if m.Offset > maxOff {
		m.Offset = maxOff
	}
}

func (m *Model) viewportH() int {
	h := m.Height - 4
	if h < 5 {
		h = 5
	}
	return h
}

func (m *Model) lines() []string {
	w := m.Width - 6
	if w < 20 {
		w = 20
	}
	body := m.Block.Body
	if body == "" {
		body = "(empty)"
	}
	return strings.Split(wordwrap.String(body, w), "\n")
}

func (m Model) View() string {
	if !m.Active {
		return ""
	}
	t := theme.Current()
	w := m.Width
	if w < 40 {
		w = 40
	}
	title := lipgloss.NewStyle().Bold(true).Foreground(t.AccentFocus).
		Render(fmt.Sprintf("▣ %s  ·  %s", m.Block.Title, m.Block.ID))
	meta := lipgloss.NewStyle().Foreground(t.TextMuted).
		Render(fmt.Sprintf("y copy body  ·  Y copy meta  ·  Esc close  ·  j/k scroll  ·  %s", m.Block.Meta))
	lines := m.lines()
	vh := m.viewportH()
	end := m.Offset + vh
	if end > len(lines) {
		end = len(lines)
	}
	var body []string
	for i := m.Offset; i < end; i++ {
		body = append(body, lines[i])
	}
	for len(body) < vh {
		body = append(body, "")
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderActive).
		Width(w - 2).
		Height(vh).
		Padding(0, 1).
		Render(strings.Join(body, "\n"))
	return lipgloss.JoinVertical(lipgloss.Left, title, meta, box)
}
