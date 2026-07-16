// Package askuser is the interactive option picker for ask_user_question.
package askuser

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// Model is the multiple-choice overlay for agent questions.
type Model struct {
	Active   bool
	Question string
	Options  []string
	// Result after user picks
	Done   bool
	Answer string // selected option text, or free-form if typed later
}

func New() Model { return Model{} }

// Open shows the question with optional choices (1–9).
func (m *Model) Open(question string, options []string) {
	m.Active = true
	m.Done = false
	m.Answer = ""
	m.Question = question
	m.Options = options
	if len(m.Options) > 9 {
		m.Options = m.Options[:9]
	}
}

func (m *Model) Close() {
	m.Active = false
	m.Done = false
}

// Pick selects option by 1-based index. Returns false if out of range.
func (m *Model) Pick(oneBased int) bool {
	if oneBased < 1 || oneBased > len(m.Options) {
		return false
	}
	m.Answer = m.Options[oneBased-1]
	m.Done = true
	m.Active = false
	return true
}

// Dismiss closes without an answer (user will type free-form).
func (m *Model) Dismiss() {
	m.Done = false
	m.Active = false
	m.Answer = ""
}

func (m Model) View() string {
	if !m.Active {
		return ""
	}
	t := theme.Current()
	w := 68
	title := lipgloss.NewStyle().Bold(true).Foreground(t.AccentFocus).Render("❓ Agent question")
	q := lipgloss.NewStyle().Foreground(t.TextPrimary).Render(wrap(m.Question, w-6))

	var body strings.Builder
	body.WriteString(title)
	body.WriteByte('\n')
	body.WriteString(q)
	body.WriteByte('\n')
	if len(m.Options) > 0 {
		body.WriteByte('\n')
		for i, o := range m.Options {
			num := lipgloss.NewStyle().Bold(true).Foreground(t.AccentUser).Render(fmt.Sprintf("%d)", i+1))
			opt := lipgloss.NewStyle().Foreground(t.TextSecondary).Render(" " + o)
			body.WriteString(num)
			body.WriteString(opt)
			body.WriteByte('\n')
		}
		body.WriteByte('\n')
		body.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render(
			"1–9 select  ·  Esc type free answer"))
	} else {
		body.WriteByte('\n')
		body.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render(
			"Esc dismiss  ·  type your answer in the prompt"))
	}
	return theme.OverlayStyle(w).Render(body.String())
}

func wrap(s string, width int) string {
	s = strings.TrimSpace(s)
	if width < 20 || len(s) <= width {
		return s
	}
	var lines []string
	for len(s) > width {
		cut := strings.LastIndex(s[:width], " ")
		if cut < 10 {
			cut = width
		}
		lines = append(lines, s[:cut])
		s = strings.TrimSpace(s[cut:])
	}
	if s != "" {
		lines = append(lines, s)
	}
	return strings.Join(lines, "\n")
}
