// Package themepicker implements Grok-style /theme live-preview overlay.
package themepicker

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/codeforge/tui/internal/theme"
)

// Model is the interactive theme list with live preview.
type Model struct {
	Active     bool
	Options    []theme.ThemeOption
	Cursor     int
	Width      int
	// SavedName is the theme active when the picker opened (for Esc revert).
	SavedName string
	WasAuto   bool
	// Done when user confirmed or cancelled.
	Done      bool
	Confirmed bool // true = Enter, false = Esc
	// PreviewName is the currently previewed theme name.
	PreviewName string
}

// New creates an inactive picker.
func New() Model {
	return Model{}
}

// Open starts the picker, saving the current theme for revert.
func (m *Model) Open() {
	m.Active = true
	m.Done = false
	m.Confirmed = false
	m.WasAuto = theme.IsAuto()
	m.SavedName = theme.DisplayName()
	if m.WasAuto {
		m.SavedName = "auto"
	}
	m.Options = theme.ThemeNamesForPicker()
	// inject auto as first option
	autoOpt := theme.ThemeOption{
		Name:        "auto",
		Description: "Follow system light/dark appearance",
		Truecolor:   false,
	}
	m.Options = append([]theme.ThemeOption{autoOpt}, m.Options...)
	// select current
	m.Cursor = 0
	cur := theme.DisplayName()
	if theme.IsAuto() {
		cur = "auto"
	}
	for i, o := range m.Options {
		if o.Name == cur {
			m.Cursor = i
			break
		}
	}
	m.previewCurrent()
}

// Close deactivates without setting Done.
func (m *Model) Close() {
	m.Active = false
	m.Done = false
}

// Move changes selection and live-previews.
func (m *Model) Move(delta int) {
	if len(m.Options) == 0 {
		return
	}
	m.Cursor += delta
	if m.Cursor < 0 {
		m.Cursor = 0
	}
	if m.Cursor >= len(m.Options) {
		m.Cursor = len(m.Options) - 1
	}
	m.previewCurrent()
}

func (m *Model) previewCurrent() {
	if m.Cursor < 0 || m.Cursor >= len(m.Options) {
		return
	}
	opt := m.Options[m.Cursor]
	m.PreviewName = opt.Name
	if opt.Name == "auto" {
		theme.EnableAuto()
		return
	}
	if opt.Factory != nil {
		theme.Set(opt.Factory())
	} else {
		theme.SetByName(opt.Name)
	}
}

// Confirm applies the previewed theme permanently.
func (m *Model) Confirm() {
	m.Done = true
	m.Confirmed = true
	m.Active = false
	// already previewed — just leave it
}

// Cancel reverts to the theme that was active when Open() was called.
func (m *Model) Cancel() {
	m.Done = true
	m.Confirmed = false
	m.Active = false
	if m.WasAuto || m.SavedName == "auto" {
		theme.EnableAuto()
	} else {
		theme.SetByName(m.SavedName)
	}
}

// Selected returns the confirmed theme name.
func (m Model) Selected() string {
	if m.Cursor >= 0 && m.Cursor < len(m.Options) {
		return m.Options[m.Cursor].Name
	}
	return ""
}

// View renders the overlay list with swatches.
func (m Model) View() string {
	if !m.Active {
		return ""
	}
	t := theme.Current()
	w := m.Width
	if w > 64 {
		w = 64
	}
	if w < 36 {
		w = 36
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(t.BorderGlow).Render("◐ Theme")
	level := theme.ColorLevelName(theme.DetectColorLevel())
	sub := lipgloss.NewStyle().Foreground(t.TextMuted).Render(
		fmt.Sprintf("color: %s  ·  ↑↓ preview  Enter apply  Esc revert", level))

	var rows []string
	for i, opt := range m.Options {
		swatch := renderSwatch(opt)
		desc := opt.Description
		if opt.Truecolor {
			desc += " · truecolor"
		}
		line := fmt.Sprintf("%s  %-12s  %s", swatch, opt.Name, desc)
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

	// Mini preview strip using current (previewed) tokens
	preview := lipgloss.JoinHorizontal(lipgloss.Left,
		lipgloss.NewStyle().Foreground(t.AccentUser).Render("user "),
		lipgloss.NewStyle().Foreground(t.AccentAssistant).Render("ai "),
		lipgloss.NewStyle().Foreground(t.AccentTool).Render("tool "),
		lipgloss.NewStyle().Foreground(t.Success).Render("ok "),
		lipgloss.NewStyle().Foreground(t.Danger).Render("err"),
	)

	body := title + "\n" + sub + "\n" +
		lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", w-6)) + "\n" +
		strings.Join(rows, "\n") + "\n" +
		lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", w-6)) + "\n" +
		"  " + preview

	return theme.OverlayStyle(w).Render(body)
}

func renderSwatch(opt theme.ThemeOption) string {
	if opt.Name == "auto" {
		return "◑◑◑"
	}
	if opt.Factory == nil {
		return "···"
	}
	tok := opt.Factory()
	// Show raw (unquantized) swatches for picker identity
	a := lipgloss.NewStyle().Foreground(tok.AccentUser).Render("●")
	b := lipgloss.NewStyle().Foreground(tok.AccentAssistant).Render("●")
	c := lipgloss.NewStyle().Foreground(tok.BgElevated).Background(tok.BgElevated).
		Foreground(tok.TextPrimary).Render("●")
	return a + b + c
}
