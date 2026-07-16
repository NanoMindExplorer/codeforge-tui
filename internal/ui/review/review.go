// Package review implements the multi-file Plan-mode approval overlay.
package review

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/ui/diffview"
)

// Model is the full-screen review UI for pending patches.
type Model struct {
	Active  bool
	Patches []tool.PendingPatch
	Cursor  int // file index
	Width   int
	Height  int
	// Result
	Done   bool
	Action string // apply | reject | cancel
}

func New() Model { return Model{} }

func (m *Model) Open(patches []tool.PendingPatch) {
	m.Active = true
	m.Patches = make([]tool.PendingPatch, len(patches))
	copy(m.Patches, patches)
	// default all accepted
	for i := range m.Patches {
		m.Patches[i].Accepted = true
	}
	m.Cursor = 0
	m.Done = false
	m.Action = ""
}

func (m *Model) Close() {
	m.Active = false
	m.Done = false
}

func (m *Model) Move(d int) {
	if len(m.Patches) == 0 {
		return
	}
	m.Cursor += d
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Patches) {
		m.Cursor = len(m.Patches) - 1
	}
}

func (m *Model) Toggle() {
	if m.Cursor >= 0 && m.Cursor < len(m.Patches) {
		m.Patches[m.Cursor].Accepted = !m.Patches[m.Cursor].Accepted
	}
}

func (m *Model) AcceptAll() {
	for i := range m.Patches {
		m.Patches[i].Accepted = true
	}
}

func (m *Model) RejectAll() {
	for i := range m.Patches {
		m.Patches[i].Accepted = false
	}
}

func (m *Model) Apply() {
	m.Done = true
	m.Active = false
	m.Action = "apply"
}

func (m *Model) Reject() {
	m.Done = true
	m.Active = false
	m.Action = "reject"
}

func (m *Model) Cancel() {
	m.Done = true
	m.Active = false
	m.Action = "cancel"
}

// Accepted returns patches marked accepted.
func (m Model) Accepted() []tool.PendingPatch {
	var out []tool.PendingPatch
	for _, p := range m.Patches {
		if p.Accepted {
			out = append(out, p)
		}
	}
	return out
}

func (m Model) View() string {
	if !m.Active {
		return ""
	}
	t := theme.Current()
	w, h := m.Width, m.Height
	if w < 40 {
		w = 40
	}
	if h < 12 {
		h = 12
	}

	header := lipgloss.NewStyle().Bold(true).Foreground(t.Warning).Render(
		"⏳ Review pending writes  ") +
		lipgloss.NewStyle().Foreground(t.TextMuted).Render(
			"j/k files · Space toggle · a all · r reject all · Enter apply · Esc cancel")

	// Split: left list 28%, right diff
	leftW := w * 28 / 100
	if leftW < 18 {
		leftW = 18
	}
	rightW := w - leftW - 4
	listH := h - 4

	var listRows []string
	listRows = append(listRows, lipgloss.NewStyle().Bold(true).Foreground(t.AccentAgent).Render("Files"))
	for i, p := range m.Patches {
		mark := " "
		if p.Accepted {
			mark = "✓"
		} else {
			mark = "✗"
		}
		add, del := diffview.CountStats(p.Diff)
		badge := lipgloss.NewStyle().Foreground(t.Success).Render(fmt.Sprintf("+%d", add)) +
			"/" + lipgloss.NewStyle().Foreground(t.Danger).Render(fmt.Sprintf("-%d", del))
		line := fmt.Sprintf("%s %s %s", mark, filepath.Base(p.RelPath), badge)
		if i == m.Cursor {
			line = lipgloss.NewStyle().Background(t.BgElevated).Foreground(t.AccentFocus).Width(leftW - 2).Render("› " + line)
		} else {
			fg := t.TextSecondary
			if !p.Accepted {
				fg = t.TextMuted
			}
			line = lipgloss.NewStyle().Foreground(fg).Width(leftW - 2).Render("  " + line)
		}
		listRows = append(listRows, line)
	}
	left := lipgloss.NewStyle().
		Width(leftW).
		Height(listH).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.BorderDim).
		Render(strings.Join(listRows, "\n"))

	var right string
	if len(m.Patches) > 0 {
		p := m.Patches[m.Cursor]
		fd := diffview.FileDiff{Path: p.RelPath, Content: p.Diff}
		right = diffview.Render(fd, rightW, listH, true)
	} else {
		right = lipgloss.NewStyle().Width(rightW).Height(listH).Render("(no patches)")
	}

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
	return lipgloss.JoinVertical(lipgloss.Left, header, body)
}
