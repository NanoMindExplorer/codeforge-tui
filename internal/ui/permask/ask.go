// Package permask is the interactive permission ask modal (y/n/always).
package permask

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// Model is the y/n/always prompt overlay.
type Model struct {
	Active    bool
	Tool      string
	Input     string
	Reason    string
	Dangerous bool
	// Result
	Done    bool
	Allow   bool
	Always  bool // remember for session/project
	// Waiting channel resolved by Confirm
	// (caller uses external channel — this model is pure UI state)
}

func New() Model { return Model{} }

func (m *Model) Open(tool, input, reason string, dangerous bool) {
	m.Active = true
	m.Done = false
	m.Allow = false
	m.Always = false
	m.Tool = tool
	m.Input = input
	m.Reason = reason
	m.Dangerous = dangerous
}

func (m *Model) Close() {
	m.Active = false
	m.Done = false
}

func (m *Model) Yes(always bool) {
	if always && m.Dangerous {
		always = false // never remember dangerous
	}
	m.Done = true
	m.Active = false
	m.Allow = true
	m.Always = always
}

func (m *Model) No(always bool) {
	if always && m.Dangerous {
		always = false
	}
	m.Done = true
	m.Active = false
	m.Allow = false
	m.Always = always
}

func (m Model) View() string {
	if !m.Active {
		return ""
	}
	t := theme.Current()
	w := 64
	title := lipgloss.NewStyle().Bold(true).Foreground(t.Warning).Render("⚠ Permission")
	tool := lipgloss.NewStyle().Foreground(t.AccentTool).Render(m.Tool)
	reason := lipgloss.NewStyle().Foreground(t.TextMuted).Render(m.Reason)
	inp := m.Input
	if len(inp) > 200 {
		inp = inp[:200] + "…"
	}
	inp = lipgloss.NewStyle().Foreground(t.TextSecondary).Render(truncateLines(inp, 6))

	keys := "y allow  ·  n deny  ·  a always allow  ·  d always deny  ·  Esc deny"
	if m.Dangerous {
		keys = "y allow once  ·  n deny  ·  Esc deny  ·  (dangerous — cannot remember)"
	}
	bar := lipgloss.NewStyle().Foreground(t.TextMuted).Render(keys)

	body := fmt.Sprintf("%s\n%s  %s\n%s\n\n%s\n\n%s",
		title, "tool:", tool, reason, inp, bar)
	return theme.OverlayStyle(w).Render(body)
}

func truncateLines(s string, max int) string {
	s = strings.TrimSpace(s)
	lines := strings.Split(s, "\n")
	if len(lines) > max {
		lines = append(lines[:max], "…")
	}
	return strings.Join(lines, "\n")
}
