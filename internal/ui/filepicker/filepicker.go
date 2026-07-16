// Package filepicker provides @file fuzzy mention UI.
package filepicker

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
	"github.com/sahilm/fuzzy"
)

// Model is a compact file picker popup.
type Model struct {
	Active   bool
	Workdir  string
	Query    string
	Files    []string
	Filtered []string
	Cursor   int
	Width    int
	Done     bool
	Selected string
}

func New(workdir string) Model {
	return Model{Workdir: workdir}
}

// Open loads non-recursive file list (excludes .git, node_modules, vendor).
func (m *Model) Open() {
	m.Active = true
	m.Query = ""
	m.Cursor = 0
	m.Done = false
	m.Selected = ""
	m.Files = listFiles(m.Workdir)
	m.Filtered = m.Files
}

func (m *Model) Close() {
	m.Active = false
	m.Done = false
}

func (m *Model) Type(s string) {
	m.Query += s
	m.refilter()
}

func (m *Model) SetQuery(q string) {
	m.Query = q
	m.refilter()
}

func (m *Model) Backspace() {
	r := []rune(m.Query)
	if len(r) == 0 {
		return
	}
	m.Query = string(r[:len(r)-1])
	m.refilter()
}

func (m *Model) refilter() {
	if m.Query == "" {
		m.Filtered = m.Files
		m.Cursor = 0
		return
	}
	matches := fuzzy.Find(m.Query, m.Files)
	m.Filtered = make([]string, 0, len(matches))
	for _, match := range matches {
		m.Filtered = append(m.Filtered, m.Files[match.Index])
	}
	m.Cursor = 0
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

func (m *Model) Confirm() {
	m.Done = true
	m.Active = false
	if len(m.Filtered) > 0 && m.Cursor < len(m.Filtered) {
		m.Selected = m.Filtered[m.Cursor]
	}
}

func (m *Model) Cancel() {
	m.Done = true
	m.Active = false
	m.Selected = ""
}

func (m Model) View() string {
	if !m.Active {
		return ""
	}
	t := theme.Current()
	w := m.Width
	if w <= 0 {
		w = 40
	}
	if w > 50 {
		w = 50
	}
	var rows []string
	rows = append(rows, lipgloss.NewStyle().Bold(true).Foreground(t.AccentUser).Render("@ file  "+m.Query+"▌"))
	max := 8
	for i, f := range m.Filtered {
		if i >= max {
			break
		}
		icon := theme.FileIcon(f)
		line := icon + " " + f
		if i == m.Cursor {
			line = lipgloss.NewStyle().Background(t.BgElevated).Foreground(t.AccentFocus).Render("› " + line)
		} else {
			line = lipgloss.NewStyle().Foreground(t.TextSecondary).Render("  " + line)
		}
		rows = append(rows, line)
	}
	if len(m.Filtered) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(t.TextMuted).Render("  (no files)"))
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.AccentUser).
		Background(t.BgOverlay).
		Padding(0, 1).
		Width(w).
		Render(strings.Join(rows, "\n"))
}

func listFiles(workdir string) []string {
	entries, err := os.ReadDir(workdir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			switch name {
			case "node_modules", "vendor", "dist", "build", ".git":
				continue
			}
			// one level deeper
			sub, err := os.ReadDir(filepath.Join(workdir, name))
			if err != nil {
				continue
			}
			for _, s := range sub {
				if s.IsDir() || strings.HasPrefix(s.Name(), ".") {
					continue
				}
				out = append(out, filepath.Join(name, s.Name()))
				if len(out) >= 200 {
					return out
				}
			}
			continue
		}
		out = append(out, name)
		if len(out) >= 200 {
			break
		}
	}
	return out
}

// ReadFileContent loads file for @mention attachment (capped).
func ReadFileContent(workdir, rel string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 32_000
	}
	path := filepath.Join(workdir, rel)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > maxBytes {
		data = data[:maxBytes]
		return string(data) + "\n… (truncated)", nil
	}
	return string(data), nil
}
