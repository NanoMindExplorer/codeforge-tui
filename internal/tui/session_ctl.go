package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/ui/components"
	"github.com/codeforge/tui/internal/ui/markdown"
)

func truncateStr(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func sessionSlugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if b.Len() > 0 {
			b.WriteByte('-')
		}
		if b.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "session"
	}
	return out
}

func (m *Model) saveSession() {
	if m.session == nil {
		return
	}
	m.session.Messages = m.chat.messages
	m.session.Provider = m.providerReg.CurrentName()
	m.session.TotalCost = m.totalCost
	m.session.Tokens = m.totalTokens
	if cur, err := m.providerReg.Current(); err == nil {
		m.session.Model = cur.Model()
	}
	_ = m.session.Save()
}

func (m *Model) persistSessionCmd() tea.Cmd {
	return func() tea.Msg {
		m.saveSession()
		return nil
	}
}

// ────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────

func (m Model) updateThemePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.themes.Cancel()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		m.onThemeApplied()
		m.toast = components.NewToast("Theme reverted", "info", 2*time.Second)
		return m, nil
	case "up", "k":
		m.themes.Move(-1)
		theme.ApplyCursorColor()
		return m, nil
	case "down", "j":
		m.themes.Move(1)
		theme.ApplyCursorColor()
		return m, nil
	case "enter":
		m.themes.Confirm()
		name := m.themes.Selected()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		m.onThemeApplied()
		m.chat.AddSystemMessage("Theme → " + theme.Name())
		m.toast = components.NewToast("Theme: "+name, "success", 2*time.Second)
		var cmds []tea.Cmd
		if theme.IsAuto() {
			cmds = append(cmds, autoThemeTick())
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// onThemeApplied refreshes derived theme state (cursor, glamour, status).
func (m *Model) onThemeApplied() {
	theme.ApplyCursorColor()
	markdown.InvalidateRenderer()
	m.status.ThemeName = theme.Name()
	m.recalcSizes()
}

// openThemePicker starts live-preview theme selection.
func (m *Model) openThemePicker() {
	if theme.MinimalMode() {
		m.chat.AddSystemMessage("Themes unavailable in --minimal mode")
		return
	}
	m.slash.Close()
	m.themes.Open()
	m.mode = ModeThemePick
	m.focusPrompt = false
	m.chat.BlurInput()
}

func (m *Model) openSessionPicker() {
	m.slash.Close()
	m.sessions.Open(m.workdir)
	m.mode = ModeSessionPick
	m.focusPrompt = false
	m.chat.BlurInput()
}

func (m *Model) openRewindPicker() {
	if m.session == nil {
		m.chat.AddSystemMessage("No session for rewind")
		return
	}
	pts, err := m.session.LoadRewindPoints()
	if err != nil || len(pts) == 0 {
		// synthesize from user messages if no recorded points
		pts = synthesizeRewindPoints(m.chat.messages)
	}
	if len(pts) == 0 {
		m.chat.AddSystemMessage("No rewind points yet — send a message first")
		return
	}
	m.slash.Close()
	m.rewinds.Open(pts)
	m.mode = ModeRewindPick
	m.focusPrompt = false
	m.chat.BlurInput()
}

func synthesizeRewindPoints(msgs []provider.Message) []session.RewindPoint {
	var pts []session.RewindPoint
	for i, msg := range msgs {
		if msg.Role != provider.RoleUser {
			continue
		}
		pts = append(pts, session.RewindPoint{
			ID:           fmt.Sprintf("synth-%d", i+1),
			CreatedAt:    time.Now().Add(-time.Duration(len(msgs)-i) * time.Minute),
			MessageIndex: i + 1,
			Preview:      msg.Content,
		})
	}
	return pts
}

func (m Model) updateSessionPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.sessions.Cancel()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		return m, nil
	case "up", "k":
		m.sessions.Move(-1)
		return m, nil
	case "down", "j":
		m.sessions.Move(1)
		return m, nil
	case "backspace":
		m.sessions.Backspace()
		return m, nil
	case "enter":
		m.sessions.Confirm()
		if m.sessions.Selected != nil {
			m.applySession(m.sessions.Selected)
		}
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		return m, nil
	default:
		if msg.Type == tea.KeyRunes {
			m.sessions.Type(string(msg.Runes))
		}
	}
	return m, nil
}

func (m Model) updateRewindPicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.rewinds.Cancel()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		return m, nil
	case "up", "k":
		m.rewinds.Move(-1)
		return m, nil
	case "down", "j":
		m.rewinds.Move(1)
		return m, nil
	case "enter":
		m.rewinds.Confirm()
		if m.rewinds.Selected != nil {
			m.applyRewind(*m.rewinds.Selected)
		}
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		return m, nil
	}
	return m, nil
}

func (m *Model) applySession(s *session.Session) {
	if s == nil {
		return
	}
	m.saveSession() // persist current before switch
	m.session = s
	m.chat.LoadMessages(s.Messages)
	m.totalCost = s.TotalCost
	m.totalTokens = s.Tokens
	if s.Provider != "" {
		_ = m.providerReg.Switch(s.Provider)
	}
	if s.Model != "" {
		if cur, err := m.providerReg.Current(); err == nil {
			_ = cur.SetModel(s.Model)
		}
	}
	m.syncWriteMode() // rebind plan.md path for this session
	m.chat.AddSystemMessage(fmt.Sprintf("✓ Resumed session %s", s.ID))
	m.toast = components.NewToast("Session resumed", "success", 2*time.Second)
}

func (m *Model) applyRewind(pt session.RewindPoint) {
	if m.session == nil {
		return
	}
	// sync messages into session first
	m.session.Messages = m.chat.messages
	restored, err := m.session.ApplyRewind(pt)
	if err != nil {
		m.chat.AddSystemMessage("⚠ rewind: " + err.Error())
		return
	}
	m.chat.LoadMessages(m.session.Messages)
	m.totalTokens = m.session.Tokens
	msg := fmt.Sprintf("✓ Rewound to %s (msg@%d)", pt.CreatedAt.Format("15:04:05"), pt.MessageIndex)
	if len(restored) > 0 {
		msg += fmt.Sprintf("\n  Restored %d file(s): %s", len(restored), strings.Join(restored, ", "))
		for _, r := range restored {
			m.context.MarkTouched(r)
		}
	}
	m.chat.AddSystemMessage(msg)
	m.toast = components.NewToast("Rewound", "success", 3*time.Second)
}

func (m *Model) startNewSession() {
	m.saveSession()
	m.chat.Clear()
	prov, model := m.providerReg.CurrentName(), ""
	if cur, err := m.providerReg.Current(); err == nil {
		model = cur.Model()
	}
	m.session = session.New(prov, model, m.workdir)
	m.totalCost = 0
	m.totalTokens = 0
	m.diff = NewDiffModel()
	m.setSessionMode(tool.SessionBuild)
	m.chat.AddSystemMessage("New session " + m.session.ID)
	m.toast = components.NewToast("New session", "info", 2*time.Second)
}

func (m *Model) maxContextTokens() int {
	if cur, err := m.providerReg.Current(); err == nil {
		// try config providers
		if m.cfg != nil {
			if p, ok := m.cfg.Providers[m.providerReg.CurrentName()]; ok && p.Capabilities.MaxContext > 0 {
				return p.Capabilities.MaxContext
			}
		}
		_ = cur
	}
	if m.cfg != nil {
		if p, ok := m.cfg.Providers[m.providerReg.CurrentName()]; ok && p.Capabilities.MaxContext > 0 {
			return p.Capabilities.MaxContext
		}
	}
	return 128000
}

func (m *Model) maybeAutoCompact() {
	maxCtx := m.maxContextTokens()
	pct := 0.85
	if m.cfg != nil && m.cfg.Session.AutoCompactPct > 0 {
		pct = m.cfg.Session.AutoCompactPct
	}
	tok := m.totalTokens
	if tok == 0 {
		tok = session.EstimateTokens(m.chat.messages)
	}
	if !session.ShouldAutoCompact(tok, maxCtx, pct) {
		return
	}
	if m.session == nil {
		return
	}
	m.session.Messages = m.chat.messages
	res, err := m.session.Compact(6, "auto-compact at context threshold")
	if err != nil {
		return
	}
	m.chat.LoadMessages(m.session.Messages)
	m.toast = components.NewToast(fmt.Sprintf("Auto-compact %d→%d msgs", res.BeforeMsgs, res.AfterMsgs), "info", 3*time.Second)
}

func (m *Model) recordTurnRewind(preview string) {
	if m.session == nil {
		return
	}
	m.session.Messages = m.chat.messages
	turn := ""
	if m.chat.store != nil {
		turn = m.chat.store.CurrentTurn()
	}
	_, _ = m.session.RecordRewindPoint(preview, turn)
}

func paneName(p Pane) string {
	switch p {
	case PaneDiff:
		return "Diff"
	case PaneContext:
		return "Files"
	default:
		return "Chat"
	}
}

// ────────────────────────────────────────────────────────────
// Slash commands
// ────────────────────────────────────────────────────────────
