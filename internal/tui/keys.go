package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/checkpoint"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/ui/blockview"
	"github.com/codeforge/tui/internal/ui/clipboard"
	"github.com/codeforge/tui/internal/ui/components"
	"github.com/codeforge/tui/internal/ui/filepicker"
	"github.com/codeforge/tui/internal/ui/palette"
	"github.com/codeforge/tui/internal/ui/planreview"
	"github.com/codeforge/tui/internal/ui/settings"
)

// handleKeyMsg owns the full keyboard mode machine (Q2.2).
// Order: global chords → steal-Esc modal stack → slash menu → focus swap → prompt/scrollback.
func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	// ── Global (after steal-Esc layers) ───────────
	switch msg.String() {
	case "ctrl+c":
		return m.handleCtrlC()
	case "ctrl+l":
		return m, tea.ClearScreen
	case "ctrl+b":
		m.showPanels = !m.showPanels
		m.recalcSizes()
		return m, nil
	case "ctrl+k":
		// steal: close slash first
		m.slash.Close()
		m.openPalette()
		return m, nil
	case "shift+tab":
		m.toggleAgentMode()
		return m, nil
	}

	// ── Steal-Esc stack (Grok order) ─────────────
	// Review → PlanReview → PermAsk → AskUser → BlockView → Settings → Theme → Resume → Rewind → …
	if m.mode == ModeReview {
		return m.updateReview(msg)
	}
	if m.mode == ModePlanReview {
		return m.updatePlanReview(msg)
	}
	if m.mode == ModePermAsk {
		return m.updatePermAsk(msg)
	}
	if m.mode == ModeAskUser {
		return m.updateAskUser(msg)
	}
	if m.mode == ModeBlockView {
		return m.updateBlockView(msg)
	}
	if m.mode == ModeSettings {
		return m.updateSettings(msg)
	}
	if m.mode == ModeThemePick {
		return m.updateThemePicker(msg)
	}
	if m.mode == ModeSessionPick {
		return m.updateSessionPicker(msg)
	}
	if m.mode == ModeRewindPick {
		return m.updateRewindPicker(msg)
	}
	if m.mode == ModePalette {
		return m.updatePalette(msg)
	}
	if m.mode == ModeFilePick {
		return m.updatePicker(msg)
	}
	if m.mode == ModeCommand {
		newCmd, c := m.command.Update(msg)
		m.command = newCmd.(CommandModel)
		if c != nil {
			cmds = append(cmds, c)
		}
		if m.command.Done {
			action := m.command.FinalValue
			m.command = NewCommandModel()
			m.focusPrompt = true
			m.mode = ModeInsert
			m.chat.FocusInput()
			if c2 := m.executeSlashCommand(action); c2 != nil {
				cmds = append(cmds, c2)
			}
		}
		m.syncStatus()
		return m, tea.Batch(cmds...)
	}

	// Slash menu navigation steals keys while active
	if m.slash.Active && m.focusPrompt {
		switch msg.String() {
		case "esc":
			m.slash.Close()
			return m, nil
		case "up", "k":
			m.slash.Move(-1)
			return m, nil
		case "down", "j":
			m.slash.Move(1)
			return m, nil
		case "tab":
			if done := m.slash.Complete(); done != "" {
				m.chat.SetInput(done)
				m.slash.UpdateQuery(done)
			}
			return m, nil
		case "enter":
			if done := m.slash.Complete(); done != "" {
				// if only command name completed, run if no more args needed
				m.chat.SetInput(done)
				m.slash.Close()
				// Run immediately if it's a no-arg style command
				cmd := strings.TrimSpace(done)
				if isImmediateSlash(cmd) {
					m.chat.ClearInput()
					if c := m.executeSlashCommand(cmd); c != nil {
						cmds = append(cmds, c)
					}
					return m, tea.Batch(cmds...)
				}
				return m, nil
			}
		}
	}

	// ── Tab: focus swap (not when slash uses Tab) ─
	if msg.String() == "tab" && !m.slash.Active {
		m.focusPrompt = !m.focusPrompt
		if m.focusPrompt {
			m.mode = ModeInsert
			m.chat.FocusInput()
		} else {
			m.mode = ModeNormal
			m.chat.BlurInput()
			m.slash.Close()
		}
		m.syncStatus()
		return m, nil
	}

	// ── PROMPT focused ───────────────────────────
	if m.focusPrompt {
		switch msg.String() {
		case "esc":
			return m.handlePromptEsc()
		case "enter":
			if m.chat.streaming {
				return m, nil
			}
			// complete slash if menu open and partial
			if m.slash.Active {
				if done := m.slash.Complete(); done != "" && !strings.Contains(strings.TrimSpace(m.chat.InputValue()), " ") {
					m.chat.SetInput(done)
				}
				m.slash.Close()
			}
			inp := strings.TrimSpace(m.chat.InputValue())
			if inp == "" {
				return m, nil
			}
			if strings.HasPrefix(inp, "/") {
				m.chat.ClearInput()
				m.slash.Close()
				if c := m.executeSlashCommand(inp); c != nil {
					cmds = append(cmds, c)
				}
				return m, tea.Batch(cmds...)
			}
			if m.budgetBlocks() {
				m.chat.AddSystemMessage("⛔ Budget exceeded — see /budget")
				return m, nil
			}
			preview := inp
			if c := m.chat.Submit(); c != nil {
				m.recordTurnRewind(preview)
				m.maybeAutoCompact()
				cmds = append(cmds, c)
				cmds = append(cmds, m.persistSessionCmd())
			}
			return m, tea.Batch(cmds...)
		case "@":
			m.slash.Close()
			m.picker.Open()
			m.mode = ModeFilePick
			return m, nil
		case "pgup", "pgdown":
			m.chat.mode = ModeNormal
			nc, c := m.chat.Update(msg)
			m.chat = nc.(ChatModel)
			m.chat.mode = ModeInsert
			if c != nil {
				cmds = append(cmds, c)
			}
			return m, tea.Batch(cmds...)
		}
		// Type into textarea then refresh slash menu
		m.mode = ModeInsert
		m.chat.mode = ModeInsert
		// Don't let slash menu steal plain typing via up/down already handled
		if msg.String() != "up" && msg.String() != "down" || !m.slash.Active {
			nc, c := m.chat.Update(msg)
			m.chat = nc.(ChatModel)
			if c != nil {
				cmds = append(cmds, c)
			}
		}
		m.slash.Width = m.width
		m.slash.UpdateQuery(m.chat.InputValue())
		m.ctrlCArmed = false
		m.syncStatus()
		return m, tea.Batch(cmds...)
	}

	// ── SCROLLBACK focused ───────────────────────
	switch msg.String() {
	case "esc":
		// double-esc with messages → rewind picker
		now := time.Now()
		if now.Sub(m.lastEsc) < 800*time.Millisecond && len(m.chat.messages) > 0 {
			m.openRewindPicker()
			m.lastEsc = time.Time{}
			return m, nil
		}
		m.lastEsc = now
		return m, nil
	case "i", "space":
		m.focusPrompt = true
		m.mode = ModeInsert
		m.chat.FocusInput()
		return m, nil
	case "/":
		m.focusPrompt = true
		m.mode = ModeInsert
		m.chat.SetInput("/")
		m.chat.FocusInput()
		m.slash.UpdateQuery("/")
		return m, nil
	case ":":
		m.mode = ModeCommand
		m.command.Activate()
		return m, nil
	case "q":
		m.saveSession()
		m.quitting = true
		return m, tea.Quit
	case "?":
		m.chat.AddSystemMessage(helpText())
		return m, nil
	case "1":
		m.showPanels = false
		m.activePane = PaneChat
		m.recalcSizes()
	case "2":
		m.showPanels = true
		m.activePane = PaneDiff
		m.recalcSizes()
	case "3":
		m.showPanels = true
		m.activePane = PaneContext
		m.recalcSizes()
	case "n", "p":
		nd, c := m.diff.Update(msg)
		m.diff = nd.(DiffModel)
		if c != nil {
			cmds = append(cmds, c)
		}
	case "enter":
		// Phase 7: fullscreen block viewer
		m.openBlockViewer()
		return m, nil
	case "y":
		m.copySelectedBlock(false)
		return m, nil
	case "Y":
		m.copySelectedBlock(true)
		return m, nil
	default:
		// Scroll keys: always arrows/pg; letter keys only if vimMode
		if m.isScrollKey(msg) {
			nc, c := m.chat.Update(msg)
			m.chat = nc.(ChatModel)
			if c != nil {
				cmds = append(cmds, c)
			}
			break
		}
		// Simple mode: printable → focus prompt
		if msg.Type == tea.KeyRunes || (len(msg.String()) == 1 && msg.String() != "q") {
			m.focusPrompt = true
			m.mode = ModeInsert
			m.chat.FocusInput()
			nc, c := m.chat.Update(msg)
			m.chat = nc.(ChatModel)
			if c != nil {
				cmds = append(cmds, c)
			}
			m.slash.UpdateQuery(m.chat.InputValue())
		}
	}

	m.syncStatus()
	return m, tea.Batch(cmds...)
}

func (m *Model) updateReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.review.Move(1)
	case "k", "up":
		m.review.Move(-1)
	case " ":
		m.review.Toggle()
	case "a":
		m.review.AcceptAll()
	case "r":
		m.review.RejectAll()
	case "enter":
		m.review.Apply()
		return m.finishReview()
	case "esc":
		m.review.Cancel()
		m.mode = ModeNormal
		return m, nil
	}
	return m, nil
}

func (m *Model) finishReview() (tea.Model, tea.Cmd) {
	sw := m.toolReg.GetStagedWriter()
	if sw == nil {
		m.mode = ModeNormal
		return m, nil
	}
	// sync acceptance state
	for i, p := range m.review.Patches {
		sw.SetAccepted(i, p.Accepted)
	}
	switch m.review.Action {
	case "apply":
		applied, d, err := sw.ApplyAccepted()
		if err != nil {
			m.chat.AddSystemMessage("⚠ apply: " + err.Error())
		} else {
			for _, a := range applied {
				_, _ = checkpoint.Save(m.session.ID, a.AbsPath, a.RelPath, a.OldContent)
				m.context.MarkTouched(a.RelPath)
			}
			if d != "" {
				nd, _ := m.diff.Update(DiffUpdateMsg{Content: d, Pending: false})
				m.diff = nd.(DiffModel)
			}
			m.toast = components.NewToast(fmt.Sprintf("Applied %d file(s)", len(applied)), "success", 3*time.Second)
			m.chat.AddSystemMessage(fmt.Sprintf("✓ Applied %d file(s) to disk", len(applied)))
		}
		sw.ClearPending()
	case "reject":
		sw.ClearPending()
		m.toast = components.NewToast("All pending writes discarded", "warning", 3*time.Second)
		m.chat.AddSystemMessage("Pending writes discarded.")
	}
	m.mode = ModeNormal
	m.activePane = PaneChat
	return m, nil
}

func (m *Model) updatePalette(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.palette.Cancel()
		m.mode = ModeNormal
		return m, nil
	case "enter":
		m.palette.Confirm()
		m.mode = ModeNormal
		if m.palette.Selected != nil {
			return m, m.handlePaletteItem(*m.palette.Selected)
		}
		return m, nil
	case "up", "k":
		m.palette.Move(-1)
	case "down", "j":
		m.palette.Move(1)
	case "backspace":
		m.palette.Backspace()
	default:
		if msg.Type == tea.KeyRunes {
			m.palette.Type(string(msg.Runes))
		}
	}
	return m, nil
}

func (m *Model) updatePicker(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.picker.Cancel()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		return m, nil
	case "enter":
		m.picker.Confirm()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		if m.picker.Selected != "" {
			sel := m.picker.Selected
			// strip range for display token; keep full sel for attach
			display := sel
			cur := m.chat.InputValue()
			// replace trailing @fragment if any
			if i := strings.LastIndex(cur, "@"); i >= 0 {
				cur = cur[:i]
			}
			m.chat.SetInput(cur + "@" + display + " ")
			if body, err := filepicker.ReadFileContent(m.workdir, sel, 32_000); err == nil {
				m.chat.AttachFile(sel, body)
				pathOnly := sel
				if i := strings.LastIndex(sel, ":"); i >= 0 {
					pathOnly = sel[:i]
				}
				m.context.MarkTouched(pathOnly)
			}
		}
		return m, nil
	case "up":
		m.picker.Move(-1)
	case "down":
		m.picker.Move(1)
	case "k":
		if m.vimMode {
			m.picker.Move(-1)
		} else {
			m.picker.Type("k")
		}
	case "j":
		if m.vimMode {
			m.picker.Move(1)
		} else {
			m.picker.Type("j")
		}
	case "backspace":
		m.picker.Backspace()
	default:
		if msg.Type == tea.KeyRunes {
			m.picker.Type(string(msg.Runes))
		}
	}
	return m, nil
}

func (m *Model) handleCtrlC() (tea.Model, tea.Cmd) {
	// Grok: 1) clear draft  2) cancel turn  3) quit
	if draft := strings.TrimSpace(m.chat.InputValue()); draft != "" {
		m.chat.PushHistory(draft)
		m.chat.ClearInput()
		m.slash.Close()
		m.ctrlCArmed = false
		m.toast = components.NewToast("Draft cleared", "info", time.Second)
		return *m, nil
	}
	if m.chat.streaming {
		m.chat.CancelTurn()
		m.streamCh = nil
		m.agentCh = nil
		m.ctrlCArmed = false
		m.toast = components.NewToast("Cancelled", "warning", 2*time.Second)
		return *m, nil
	}
	if m.ctrlCArmed {
		m.saveSession()
		m.quitting = true
		return *m, tea.Quit
	}
	m.ctrlCArmed = true
	m.toast = components.NewToast("Ctrl+C again to quit", "info", 2*time.Second)
	return *m, nil
}

func (m *Model) handlePromptEsc() (tea.Model, tea.Cmd) {
	// steal: slash menu
	if m.slash.Active {
		m.slash.Close()
		return *m, nil
	}
	now := time.Now()
	draft := strings.TrimSpace(m.chat.InputValue())
	if draft != "" {
		if now.Sub(m.lastEsc) < 800*time.Millisecond {
			m.chat.PushHistory(draft)
			m.chat.ClearInput()
			m.slash.Close()
			m.lastEsc = time.Time{}
			m.toast = components.NewToast("Prompt cleared", "info", time.Second)
			return *m, nil
		}
		m.lastEsc = now
		m.toast = components.NewToast("Esc again to clear", "info", time.Second)
		return *m, nil
	}
	// empty prompt → double-esc opens rewind picker
	if now.Sub(m.lastEsc) < 800*time.Millisecond && len(m.chat.messages) > 0 {
		m.openRewindPicker()
		m.lastEsc = time.Time{}
		return *m, nil
	}
	m.lastEsc = now
	m.focusPrompt = false
	m.mode = ModeNormal
	m.chat.BlurInput()
	m.slash.Close()
	return *m, nil
}

func (m *Model) isScrollKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "up", "down", "left", "right", "pgup", "pgdown", "ctrl+d", "ctrl+u":
		return true
	case "j", "k", "h", "l", "g", "G", "e", "E":
		return m.vimMode
	default:
		return false
	}
}

func (m *Model) openPalette() {
	items := buildPaletteItems(m)
	m.palette.Open(items)
	m.mode = ModePalette
}

func (m *Model) handlePaletteItem(it palette.Item) tea.Cmd {
	switch {
	case strings.HasPrefix(it.ID, "cmd:"):
		cmd := strings.TrimPrefix(it.ID, "cmd:")
		return m.executeSlashCommand(cmd)
	case strings.HasPrefix(it.ID, "file:"):
		rel := strings.TrimPrefix(it.ID, "file:")
		m.mode = ModeInsert
		m.chat.mode = ModeInsert
		m.chat.SetInput("@" + rel + " ")
		m.chat.FocusInput()
		if body, err := filepicker.ReadFileContent(m.workdir, rel, 32_000); err == nil {
			m.chat.AttachFile(rel, body)
		}
	case strings.HasPrefix(it.ID, "session:"):
		id := strings.TrimPrefix(it.ID, "session:")
		return m.executeSlashCommand("sessions " + id)
	}
	return nil
}

func buildPaletteItems(m *Model) []palette.Item {
	var items []palette.Item
	cmds := []struct{ id, label, desc string }{
		{"/act", "Act (agent)", "Start agent task"},
		{"/help", "Help", "Show keybindings"},
		{"/mode", "Mode", "Show/switch Plan|Act"},
		{"/sessions", "Sessions", "List saved sessions"},
		{"/undo", "Undo", "Restore last write"},
		{"/provider", "Provider", "Switch AI provider"},
		{"/model", "Model", "Switch model"},
		{"/cost", "Cost", "Session cost"},
		{"/status", "Git status", "Show git status"},
		{"/commit", "Commit", "Stage all + commit"},
		{"/push", "Push", "Push branch to origin"},
		{"/pull", "Pull", "Pull from origin"},
		{"/pr list", "PR list", "List pull requests"},
		{"/pr create", "PR create", "Create pull request"},
		{"/pr babysit", "PR babysit", "Poll CI until green"},
		{"/pr babysit --fix", "PR babysit+fix", "Poll CI then agent-fix"},
		{"/issue list", "Issues", "List GitHub issues"},
		{"/gh auth", "GitHub auth", "Auth status"},
		{"/rules", "Rules", "Show project AGENTS.md rules"},
		{"/index", "Index", "Codebase index stats"},
		{"/budget", "Budget", "Cost budget status"},
		{"/clear", "Clear", "Clear chat"},
		{"/quit", "Quit", "Exit CodeForge"},
	}
	for _, c := range cmds {
		items = append(items, palette.Item{
			ID: "cmd:" + c.id, Label: c.label, Description: c.desc, Category: "command",
		})
	}
	// files
	entries, _ := os.ReadDir(m.workdir)
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		items = append(items, palette.Item{
			ID: "file:" + e.Name(), Label: e.Name(), Description: "project file", Category: "file",
		})
	}
	// sessions
	if sess, err := session.List(10); err == nil {
		for _, s := range sess {
			items = append(items, palette.Item{
				ID: "session:" + s.ID, Label: s.ID + " " + s.Slug,
				Description: s.Preview, Category: "session",
			})
		}
	}
	return items
}

// cycleSessionMode: BUILD → DESIGN → YOLO → BUILD (Shift+Tab / Grok parity).
func (m *Model) cycleSessionMode() {
	m.setSessionMode(m.sessionMode.Next())
}

func (m *Model) setSessionMode(mode tool.SessionMode) {
	m.sessionMode = mode
	m.syncWriteMode()
	m.syncPermWithSession()
	switch mode {
	case tool.SessionDesign:
		m.toast = components.NewToast("Mode: DESIGN — plan only", "info", 3*time.Second)
		m.chat.AddSystemMessage("◈ DESIGN: explore + write_plan only. No project file edits until you approve.\n  Finish with exit_plan_mode · or /view-plan")
	case tool.SessionYolo:
		m.toast = components.NewToast("Mode: YOLO — writes immediate", "warning", 3*time.Second)
		m.chat.AddSystemMessage("⚡ YOLO: write_file applies to disk immediately (always-approve).")
	default:
		m.toast = components.NewToast("Mode: BUILD — staged writes", "info", 3*time.Second)
		m.chat.AddSystemMessage("🛡 BUILD: write_file is staged for review before apply.")
	}
}

// syncWriteMode pushes session mode + plan path into the tool registry.
func (m *Model) syncWriteMode() {
	sw := m.toolReg.GetStagedWriter()
	if sw == nil {
		return
	}
	sw.SetMode(m.sessionMode.WriteMode())
	if m.session != nil {
		if p, err := m.session.PlanPath(); err == nil {
			sw.SetPlanPath(p)
		}
	}
}

// legacy alias used by older call sites
func (m *Model) toggleAgentMode() { m.cycleSessionMode() }

func (m *Model) openBlockViewer() {
	b := m.chat.store.Selected()
	if b == nil {
		m.toast = components.NewToast("No block selected", "info", 2*time.Second)
		return
	}
	m.blockV.Open(blockview.Content{
		ID: b.ID, Title: b.Title, Body: b.Body, Meta: b.Meta,
	})
	m.mode = ModeBlockView
	m.focusPrompt = false
	m.chat.BlurInput()
}

func (m *Model) copySelectedBlock(meta bool) {
	var text string
	if meta {
		text = m.chat.store.SelectedMeta()
	} else {
		text = m.chat.store.SelectedBody()
	}
	if text == "" {
		// fallback: last assistant message
		for i := len(m.chat.messages) - 1; i >= 0; i-- {
			if m.chat.messages[i].Role == provider.RoleAssistant {
				text = m.chat.messages[i].Content
				break
			}
		}
	}
	if text == "" {
		m.toast = components.NewToast("Nothing to copy", "warning", 2*time.Second)
		return
	}
	if err := clipboard.Write(text); err != nil {
		m.chat.AddSystemMessage("Copy: " + err.Error())
		m.toast = components.NewToast("Copied to file/fallback", "info", 2*time.Second)
		return
	}
	label := "body"
	if meta {
		label = "meta"
	}
	m.toast = components.NewToast("Copied "+label, "success", 2*time.Second)
}

func (m Model) updateBlockView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.blockV.Close()
		m.mode = ModeNormal
		m.focusPrompt = false
		return m, nil
	case "j", "down":
		m.blockV.Scroll(1)
	case "k", "up":
		m.blockV.Scroll(-1)
	case "pgdown":
		m.blockV.Scroll(10)
	case "pgup":
		m.blockV.Scroll(-10)
	case "y":
		_ = clipboard.Write(m.blockV.Block.Body)
		m.toast = components.NewToast("Copied body", "success", 2*time.Second)
	case "Y":
		meta := fmt.Sprintf("id=%s title=%q meta=%q", m.blockV.Block.ID, m.blockV.Block.Title, m.blockV.Block.Meta)
		_ = clipboard.Write(meta)
		m.toast = components.NewToast("Copied meta", "success", 2*time.Second)
	}
	return m, nil
}

func (m *Model) openSettings() {
	vim := "off"
	if m.vimMode {
		vim = "on"
	}
	compact := "off"
	if theme.CompactMode() {
		compact = "on"
	}
	permMode := "default"
	if m.perm != nil {
		permMode = string(m.perm.GetMode())
	}
	modelName := ""
	if cur, err := m.providerReg.Current(); err == nil {
		modelName = cur.Model()
	}
	m.settings.Open([]settings.Row{
		{Key: "theme", Value: theme.Name(), Hint: "Enter → picker"},
		{Key: "vim_mode", Value: vim, Hint: "toggle"},
		{Key: "compact_mode", Value: compact, Hint: "toggle"},
		{Key: "session_mode", Value: m.sessionMode.Label(), Hint: "cycle"},
		{Key: "permission_mode", Value: permMode, Hint: "cycle"},
		{Key: "sandbox", Value: string(m.sandboxEng().Profile), Hint: "/sandbox"},
		{Key: "provider", Value: m.providerReg.CurrentName(), Hint: modelName},
		{Key: "todos", Value: m.todosList().Badge(), Hint: "/todos"},
		{Key: "bg_tasks", Value: fmt.Sprintf("%d running", m.bgTasks().RunningCount()), Hint: "/tasks"},
	})
	m.mode = ModeSettings
	m.focusPrompt = false
	m.chat.BlurInput()
}

func (m Model) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.settings.Close()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		return m, nil
	case "up", "k":
		m.settings.Move(-1)
	case "down", "j":
		m.settings.Move(1)
	case "enter", " ":
		m.settings.Activate()
		key := m.settings.Action
		switch key {
		case "theme":
			m.settings.Close()
			m.openThemePicker()
			return m, nil
		case "vim_mode":
			m.vimMode = !m.vimMode
		case "compact_mode":
			theme.ToggleCompact()
			m.recalcSizes()
		case "session_mode":
			m.cycleSessionMode()
		case "permission_mode":
			if m.perm != nil {
				switch m.perm.GetMode() {
				case permission.ModeDefault:
					m.perm.SetMode(permission.ModeAlwaysApprove)
				case permission.ModeAlwaysApprove:
					m.perm.SetMode(permission.ModeDontAsk)
				case permission.ModeDontAsk:
					m.perm.SetMode(permission.ModePlan)
				default:
					m.perm.SetMode(permission.ModeDefault)
				}
			}
		case "todos":
			m.settings.Close()
			m.chat.AddSystemMessage(m.todosList().Render())
			m.mode = ModeInsert
			m.focusPrompt = true
			m.chat.FocusInput()
			return m, nil
		case "bg_tasks":
			m.settings.Close()
			m.chat.AddSystemMessage(m.bgTasks().Summary())
			m.mode = ModeInsert
			m.focusPrompt = true
			m.chat.FocusInput()
			return m, nil
		}
		// refresh rows
		m.openSettings()
	}
	return m, nil
}

func (m Model) updatePermAsk(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	reply := func(allow, always bool) (tea.Model, tea.Cmd) {
		if m.permWait != nil && m.permWait.Reply != nil {
			select {
			case m.permWait.Reply <- permAskReply{Allow: allow, Always: always}:
			default:
			}
		}
		m.permWait = nil
		m.permAsk.Close()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		return m, listenPermAsk(m.permReq)
	}
	switch msg.String() {
	case "y", "Y":
		return reply(true, false)
	case "n", "N", "esc":
		return reply(false, false)
	case "a", "A":
		if m.permAsk.Dangerous {
			return reply(true, false)
		}
		return reply(true, true)
	case "d", "D":
		if m.permAsk.Dangerous {
			return reply(false, false)
		}
		return reply(false, true)
	}
	return m, nil
}

func (m Model) updateAskUser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.userAsk.Dismiss()
		m.mode = ModeInsert
		m.focusPrompt = true
		m.chat.FocusInput()
		m.toast = components.NewToast("Type your answer in the prompt", "info", 3*time.Second)
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		n, _ := strconv.Atoi(msg.String())
		if m.userAsk.Pick(n) {
			ans := m.userAsk.Answer
			m.userAsk.Close()
			m.mode = ModeInsert
			m.focusPrompt = true
			m.chat.FocusInput()
			// Submit selected option as the next user turn
			m.chat.SetInput(ans)
			if c := m.chat.Submit(); c != nil {
				m.toast = components.NewToast("Answered: "+truncateStr(ans, 40), "success", 2*time.Second)
				return m, tea.Batch(c, m.persistSessionCmd())
			}
		}
		return m, nil
	}
	return m, nil
}

// syncPermWithSession mirrors YOLO/DESIGN into permission engine mode.
func (m *Model) syncPermWithSession() {
	if m.perm == nil {
		return
	}
	switch m.sessionMode {
	case tool.SessionYolo:
		m.perm.SetMode(permission.ModeAlwaysApprove)
	case tool.SessionDesign:
		m.perm.SetMode(permission.ModePlan)
	default:
		// keep config mode if default; don't force away from dont_ask
		if m.perm.GetMode() == permission.ModeAlwaysApprove || m.perm.GetMode() == permission.ModePlan {
			m.perm.SetMode(permission.ModeDefault)
		}
	}
}

func (m *Model) openPlanReview(summary string) {
	content := ""
	if m.session != nil {
		content, _ = m.session.ReadPlan()
	}
	m.planUI.Open(content, summary)
	m.mode = ModePlanReview
	m.focusPrompt = false
	m.chat.BlurInput()
	m.slash.Close()
}

func (m Model) updatePlanReview(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.planUI.Quit()
		return m.finishPlanReview()
	case "a":
		if m.planUI.Focus == planreview.FocusPrompt {
			// approve with notes
			m.planUI.Approve()
			return m.finishPlanReview()
		}
		m.planUI.Approve()
		return m.finishPlanReview()
	case "s":
		m.planUI.RequestChanges()
		if m.planUI.Done {
			return m.finishPlanReview()
		}
		return m, nil
	case "tab":
		m.planUI.ToggleFocus()
		return m, nil
	case "esc":
		if m.planUI.Focus == planreview.FocusPrompt {
			m.planUI.Focus = planreview.FocusPreview
			return m, nil
		}
		// Esc on preview does nothing (must q to quit) — match Grok
		return m, nil
	case "up", "k":
		if m.planUI.Focus == planreview.FocusPreview {
			m.planUI.Scroll(-1)
		}
		return m, nil
	case "down", "j":
		if m.planUI.Focus == planreview.FocusPreview {
			m.planUI.Scroll(1)
		}
		return m, nil
	case "pgup":
		m.planUI.Page(-1)
		return m, nil
	case "pgdown":
		m.planUI.Page(1)
		return m, nil
	case "enter":
		if m.planUI.Focus == planreview.FocusPrompt {
			m.planUI.SubmitChanges()
			return m.finishPlanReview()
		}
		return m, nil
	case "backspace":
		if m.planUI.Focus == planreview.FocusPrompt {
			m.planUI.BackspaceFeedback()
		}
		return m, nil
	default:
		if m.planUI.Focus == planreview.FocusPrompt && msg.Type == tea.KeyRunes {
			m.planUI.TypeFeedback(string(msg.Runes))
		}
	}
	return m, nil
}

func (m Model) finishPlanReview() (tea.Model, tea.Cmd) {
	action := m.planUI.Action
	notes := m.planUI.Notes
	m.planUI.Close()
	m.mode = ModeInsert
	m.focusPrompt = true
	m.chat.FocusInput()

	switch action {
	case planreview.ActionApprove:
		// Leave DESIGN → BUILD for implementation
		m.setSessionMode(tool.SessionBuild)
		task := "The design plan was APPROVED. Implement it now.\n"
		if body, err := m.session.ReadPlan(); err == nil && body != "" {
			task += "\n--- plan.md ---\n" + body + "\n--- end plan ---\n"
		}
		if notes != "" {
			task += "\nUser comments with approval:\n" + notes + "\n"
		}
		task += "\nUse search_replace/apply_patch. Prefer BUILD staged writes."
		m.chat.AddSystemMessage("✓ Plan approved → BUILD mode. Starting implementation…")
		m.toast = components.NewToast("Plan approved", "success", 3*time.Second)
		return m, m.chat.SubmitAgent(task)

	case planreview.ActionChanges:
		// Stay in DESIGN for revision
		if m.sessionMode != tool.SessionDesign {
			m.setSessionMode(tool.SessionDesign)
		}
		task := "The user requested CHANGES to the design plan.\nFeedback:\n" + notes +
			"\n\nRevise the plan with write_plan, then call exit_plan_mode again. Do not implement yet."
		m.chat.AddSystemMessage("↩ Changes requested — still in DESIGN")
		m.toast = components.NewToast("Revise plan", "info", 3*time.Second)
		return m, m.chat.SubmitAgent(task)

	case planreview.ActionQuit:
		m.setSessionMode(tool.SessionBuild)
		m.chat.AddSystemMessage("Plan abandoned → BUILD mode")
		m.toast = components.NewToast("Plan quit", "warning", 2*time.Second)
		return m, nil
	}
	return m, nil
}
