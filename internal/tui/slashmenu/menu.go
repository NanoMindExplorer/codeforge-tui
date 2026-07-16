// Package slashmenu implements Grok-style / command autocomplete dropdown.
package slashmenu

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
	"github.com/sahilm/fuzzy"
)

// Item is one slash command.
type Item struct {
	Command string // e.g. "/act"
	Desc    string
}

// Model is the dropdown state.
type Model struct {
	Active   bool
	Query    string // full input including leading /
	Items    []Item
	Filtered []Item
	Cursor   int
	Width    int
}

// DefaultItems returns built-in CodeForge slash commands.
func DefaultItems() []Item {
	return []Item{
		{"/act", "Start agent task with tools"},
		{"/read", "Read a file via agent"},
		{"/ls", "List directory"},
		{"/grep", "Search project"},
		{"/run", "Run shell command via agent"},
		{"/explain", "Explain a file"},
		{"/fix", "Find and fix bugs"},
		{"/status", "Git status"},
		{"/commit", "Stage all + commit"},
		{"/push", "Push to origin"},
		{"/pull", "Pull from origin"},
		{"/pr", "Pull requests (list/create/…)"},
		{"/issue", "GitHub issues"},
		{"/gh", "GitHub hub"},
		{"/provider", "Switch AI provider"},
		{"/model", "Switch model"},
		{"/mode", "BUILD / DESIGN / YOLO session mode"},
		{"/plan", "Enter DESIGN plan mode"},
		{"/view-plan", "Review design plan"},
		{"/permissions", "Permission mode & rules"},
		{"/hooks", "List loaded hooks"},
		{"/cost", "Session token cost"},
		{"/budget", "Budget status"},
		{"/rules", "Show project rules"},
		{"/index", "Codebase index stats"},
		{"/theme", "Theme picker (live preview)"},
		{"/compact-mode", "Toggle compact UI"},
		{"/vim-mode", "Toggle vim scrollback keys"},
		{"/resume", "Resume session picker"},
		{"/new", "New session (new id)"},
		{"/fork", "Fork conversation"},
		{"/rewind", "Rewind files + chat"},
		{"/compact", "Compress conversation history"},
		{"/context", "Token / context breakdown"},
		{"/session-info", "Current session details"},
		{"/sessions", "List sessions"},
		{"/undo", "Undo last write"},
		{"/clear", "Clear chat (keep session id)"},
		{"/help", "Help"},
		{"/about", "About"},
		{"/quit", "Exit CodeForge"},
	}
}

func New() Model {
	return Model{Items: DefaultItems()}
}

// UpdateQuery refreshes filter from full prompt text.
func (m *Model) UpdateQuery(input string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		m.Active = false
		m.Query = ""
		return
	}
	// only first token drives menu
	tok := input
	if i := strings.IndexAny(input, " \t"); i > 0 {
		tok = input[:i]
	}
	m.Query = tok
	m.Active = true
	m.refilter()
}

func (m *Model) refilter() {
	q := strings.TrimPrefix(m.Query, "/")
	if q == "" {
		m.Filtered = m.Items
		m.Cursor = 0
		return
	}
	labels := make([]string, len(m.Items))
	for i, it := range m.Items {
		labels[i] = it.Command + " " + it.Desc
	}
	matches := fuzzy.Find(q, labels)
	m.Filtered = make([]Item, 0, len(matches))
	for _, match := range matches {
		m.Filtered = append(m.Filtered, m.Items[match.Index])
	}
	// also prefix match boost already handled by fuzzy
	if m.Cursor >= len(m.Filtered) {
		m.Cursor = 0
	}
}

func (m *Model) Move(d int) {
	if len(m.Filtered) == 0 {
		return
	}
	m.Cursor += d
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Filtered) {
		m.Cursor = len(m.Filtered) - 1
	}
}

// Selected returns the command string without trailing space, or "".
func (m *Model) Selected() string {
	if !m.Active || len(m.Filtered) == 0 {
		return ""
	}
	if m.Cursor < 0 || m.Cursor >= len(m.Filtered) {
		return ""
	}
	return m.Filtered[m.Cursor].Command
}

// Complete returns command + space for insertion into prompt.
func (m *Model) Complete() string {
	s := m.Selected()
	if s == "" {
		return ""
	}
	return s + " "
}

func (m *Model) Close() {
	m.Active = false
	m.Query = ""
	m.Cursor = 0
}

// View renders dropdown above the prompt (call before prompt join).
func (m Model) View() string {
	if !m.Active || len(m.Filtered) == 0 {
		return ""
	}
	t := theme.Current()
	w := m.Width
	if w < 40 {
		w = 40
	}
	if w > 72 {
		w = 72
	}
	maxShow := 8
	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(t.AccentUser).Render("  / commands"))
	for i, it := range m.Filtered {
		if i >= maxShow {
			rows = append(rows, lipgloss.NewStyle().Foreground(t.TextMuted).Render(
				"  … +more"))
			break
		}
		line := it.Command + "  " + it.Desc
		if i == m.Cursor {
			line = lipgloss.NewStyle().
				Background(t.BgElevated).
				Foreground(t.AccentFocus).
				Width(w - 4).
				Render("› " + line)
		} else {
			line = lipgloss.NewStyle().
				Foreground(t.TextSecondary).
				Width(w - 4).
				Render("  " + line)
		}
		rows = append(rows, line)
	}
	rows = append(rows, lipgloss.NewStyle().Foreground(t.TextMuted).Italic(true).Render(
		"  ↑↓  Tab/Enter complete  Esc close"))
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderGlow).
		Background(t.BgOverlay).
		Width(w).
		Padding(0, 1).
		Render(strings.Join(rows, "\n"))
}
