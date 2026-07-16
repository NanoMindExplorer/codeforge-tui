package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/git"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/tui/blocks"
)

// ChatModel is the Grok-style conversation + composer.
// Phase 1: scrollback is a block store (foldable, selectable, follow-tail).
type ChatModel struct {
	providerReg *provider.Registry
	toolReg     *tool.Registry
	gitRepo     *git.Repo
	workdir     string

	width  int
	height int // total area assigned by parent (scrollback portion for SetSize)

	// scrollback
	store *blocks.Store
	// composer
	ta    textarea.Model
	useTA bool
	ready bool

	streaming bool
	mode      Mode

	messages   []provider.Message
	agentFull  string
	streamFull string
	spinnerFrame int

	// typewriter for system messages
	typewriterQ  []string
	typewriterOn bool

	// input history (↑)
	history    []string
	historyIdx int

	attachments map[string]string
	rulesText   string

	// cancelFn cancels in-flight stream/agent (Grok Ctrl+C)
	cancelFn context.CancelFunc

	// Auth gates tool calls (Phase 6 permissions); optional.
	Auth agent.Authorizer
}

func NewChatModel(provReg *provider.Registry, toolReg *tool.Registry, repo *git.Repo, workdir string) ChatModel {
	ta := textarea.New()
	ta.Placeholder = "message, /command, or @file…"
	ta.ShowLineNumbers = false
	ta.CharLimit = 32_000
	ta.SetHeight(3)
	ta.Prompt = "❯ "
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()

	store := blocks.NewStore()
	c := ChatModel{
		providerReg: provReg,
		toolReg:     toolReg,
		gitRepo:     repo,
		workdir:     workdir,
		store:       store,
		ta:          ta,
		attachments: make(map[string]string),
	}
	c.store.AddSystem("CodeForge · Grok-parity  ·  type to chat  ·  / commands  ·  ? help")
	c.store.AddSystem("Scrollback: j/k select · h/l fold · E expand-all · G follow  ·  Tab focus prompt")
	c.store.AddSystem("Modes: BUILD (staged) · DESIGN (plan) · YOLO  ·  Shift+Tab cycle")
	return c
}

func (c ChatModel) Init() tea.Cmd { return textarea.Blink }

// Store exposes the block engine (tests / model).
func (c *ChatModel) Store() *blocks.Store { return c.store }

func (c *ChatModel) SetSize(w, h int) {
	c.width = w
	c.height = h
	// entire h is scrollback in Grok layout (prompt sized by parent)
	c.store.SetSize(w, h)
	innerW := w - 4
	if innerW < 10 {
		innerW = 10
	}
	c.ta.SetWidth(innerW)
	c.ta.SetHeight(3)
	c.useTA = true
	c.ready = true
}

func (c *ChatModel) InputValue() string {
	if c.useTA {
		return c.ta.Value()
	}
	return ""
}

func (c *ChatModel) SetInput(s string) { c.ta.SetValue(s) }
func (c *ChatModel) ClearInput()       { c.ta.Reset() }
func (c *ChatModel) FocusInput()       { c.ta.Focus() }
func (c *ChatModel) BlurInput()        { c.ta.Blur() }
func (c *ChatModel) InputFocused() bool { return c.ta.Focused() }

func (c *ChatModel) AttachFile(rel, content string) {
	if c.attachments == nil {
		c.attachments = make(map[string]string)
	}
	c.attachments[rel] = content
}

func (c *ChatModel) SetRules(text string) { c.rulesText = text }

func (c *ChatModel) systemWithRules(base string) string {
	if c.rulesText == "" {
		return base
	}
	return rules.Inject(base, &rules.Bundle{Text: c.rulesText})
}

// Submit streaming chat.
func (c *ChatModel) Submit() tea.Cmd {
	userMsg := strings.TrimSpace(c.ta.Value())
	if userMsg == "" || c.streaming {
		return nil
	}
	if strings.HasPrefix(userMsg, "/") {
		return nil
	}
	c.history = append(c.history, userMsg)
	c.historyIdx = len(c.history)
	c.ta.Reset()

	fullContent := userMsg
	if len(c.attachments) > 0 {
		var att strings.Builder
		att.WriteString(userMsg)
		att.WriteString("\n\n--- attached files ---\n")
		for name, body := range c.attachments {
			att.WriteString(fmt.Sprintf("\n### %s\n```\n%s\n```\n", name, body))
		}
		fullContent = att.String()
		c.attachments = make(map[string]string)
	}

	c.store.AddUser(userMsg)
	c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: fullContent})
	c.streaming = true
	c.streamFull = ""
	c.store.GotoBottom()

	msgs := make([]provider.Message, len(c.messages))
	copy(msgs, c.messages)
	prov := c.providerReg
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelFn = cancel

	return func() tea.Msg {
		p, err := prov.Current()
		if err != nil {
			return errMsg{err: fmt.Errorf("no provider: %w", err)}
		}
		req := provider.CompletionRequest{
			Messages:  msgs,
			MaxTokens: 4096,
			System:    c.systemWithRules(systemPrompt),
		}
		ch, err := p.Stream(ctx, req)
		if err != nil {
			return errMsg{err: err}
		}
		first, ok := <-ch
		if !ok {
			return StreamOpenedMsg{Ch: nil}
		}
		return StreamOpenedMsg{Ch: ch, FirstToken: first}
	}
}

func (c *ChatModel) SubmitAgent(task string) tea.Cmd {
	task = strings.TrimSpace(task)
	if task == "" || c.streaming {
		return nil
	}
	c.history = append(c.history, "/act "+task)
	c.historyIdx = len(c.history)

	if len(c.attachments) > 0 {
		var att strings.Builder
		att.WriteString(task)
		att.WriteString("\n\n--- attached files ---\n")
		for name, body := range c.attachments {
			att.WriteString(fmt.Sprintf("\n### %s\n```\n%s\n```\n", name, body))
		}
		task = att.String()
		c.attachments = make(map[string]string)
	}

	c.store.AddUser("🤖 " + task)
	c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: task})
	c.streaming = true
	c.agentFull = ""
	c.store.GotoBottom()

	msgs := make([]provider.Message, len(c.messages))
	copy(msgs, c.messages)
	prov := c.providerReg
	toolReg := c.toolReg
	ctx, cancel := context.WithCancel(context.Background())
	c.cancelFn = cancel

	return func() tea.Msg {
		p, err := prov.Current()
		if err != nil {
			return errMsg{err: fmt.Errorf("no provider: %w", err)}
		}
		cfg := agent.Config{
			Provider:  p,
			Tools:     toolReg,
			System:    c.systemWithRules(agentSystemPrompt),
			MaxTokens: 4096,
			Auth:      c.Auth,
		}
		ch := agent.Run(ctx, cfg, msgs)
		first, ok := <-ch
		if !ok {
			return AgentOpenedMsg{Ch: nil, First: agent.Event{Kind: agent.EventDone}}
		}
		return AgentOpenedMsg{Ch: ch, First: first}
	}
}

// CancelTurn aborts in-flight stream/agent (Grok second Ctrl+C).
func (c *ChatModel) CancelTurn() {
	if c.cancelFn != nil {
		c.cancelFn()
		c.cancelFn = nil
	}
	if c.streaming {
		c.store.SealAssistant()
		c.store.AddSystem("⏹ cancelled")
		c.streaming = false
		c.streamFull = ""
		c.agentFull = ""
	}
}

// PushHistory saves a cleared draft to prompt history (double-Esc).
func (c *ChatModel) PushHistory(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	c.history = append(c.history, s)
	c.historyIdx = len(c.history)
}

func (c ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SpinnerTickMsg:
		if c.streaming {
			c.spinnerFrame = (c.spinnerFrame + 1) % len(spinnerFrames)
		}
		if c.typewriterOn && len(c.typewriterQ) > 0 {
			for i := 0; i < 2 && len(c.typewriterQ) > 0; i++ {
				c.store.AddSystem(c.typewriterQ[0])
				c.typewriterQ = c.typewriterQ[1:]
			}
			if len(c.typewriterQ) == 0 {
				c.typewriterOn = false
			}
		}

	case StreamTickMsg:
		if msg.Error != nil {
			c.store.SealAssistant()
			c.store.AddSystem("⚠ Error: " + msg.Error.Error())
			c.streaming = false
			c.streamFull = ""
			break
		}
		if msg.Text != "" {
			c.store.AppendAssistantChunk(msg.Text)
			c.streamFull += msg.Text
		}
		if msg.Done {
			c.store.SealAssistant()
			if c.streamFull != "" {
				c.messages = append(c.messages, provider.Message{
					Role: provider.RoleAssistant, Content: c.streamFull,
				})
			}
			if msg.InputTokens > 0 || msg.OutputTokens > 0 {
				c.store.AddSystem(fmt.Sprintf("tokens: %d in / %d out", msg.InputTokens, msg.OutputTokens))
			}
			c.streaming = false
			c.streamFull = ""
		}

	case AgentEventMsg:
		ev := msg.Ev
		switch ev.Kind {
		case agent.EventText:
			c.store.AppendAssistantChunk(ev.Text)
			c.agentFull += ev.Text
		case agent.EventToolCall:
			c.store.SealAssistant()
			c.store.AddToolCall(ev.ToolName, truncate(ev.ToolInput, 120))
		case agent.EventToolProgress:
			txt := strings.TrimSpace(ev.Progress)
			if txt != "" {
				c.store.AddToolProgress(truncate(txt, 100))
			}
		case agent.EventToolResult:
			c.store.AddToolResult(ev.ToolName, truncate(ev.ToolOutput, 2000), ev.ToolSuccess)
			if ev.ToolDiff != "" {
				c.store.AddDiff(ev.ToolName, ev.ToolDiff, "")
			}
		case agent.EventDone:
			c.store.SealAssistant()
			if c.agentFull != "" {
				c.messages = append(c.messages, provider.Message{
					Role: provider.RoleAssistant, Content: c.agentFull,
				})
			}
			if ev.InputTokens > 0 || ev.OutputTokens > 0 {
				c.store.AddSystem(fmt.Sprintf("tokens: %d in / %d out", ev.InputTokens, ev.OutputTokens))
			}
			c.streaming = false
			c.agentFull = ""
		case agent.EventError:
			c.store.SealAssistant()
			c.store.AddSystem("⚠ agent: " + ev.Error.Error())
			c.streaming = false
			c.agentFull = ""
		}

	case tea.KeyMsg:
		if c.mode == ModeInsert && !c.streaming {
			switch msg.String() {
			case "up":
				if c.ta.Value() == "" || c.historyIdx < len(c.history) {
					if c.historyIdx > 0 {
						c.historyIdx--
						if c.historyIdx < len(c.history) {
							c.ta.SetValue(c.history[c.historyIdx])
							c.ta.CursorEnd()
						}
					}
					return c, nil
				}
			case "down":
				if c.historyIdx < len(c.history)-1 {
					c.historyIdx++
					c.ta.SetValue(c.history[c.historyIdx])
					c.ta.CursorEnd()
					return c, nil
				} else if c.historyIdx == len(c.history)-1 {
					c.historyIdx = len(c.history)
					c.ta.Reset()
					return c, nil
				}
			}
		}
		// Scrollback navigation when Normal (keys filtered by model for vim mode)
		if c.mode == ModeNormal {
			c.store.SetShowSelection(true)
			switch msg.String() {
			case "j", "down":
				c.store.SelectNext()
			case "k", "up":
				c.store.SelectPrev()
			case "h", "left":
				idx := c.store.SelectedIndex()
				bl := c.store.Blocks()
				if idx >= 0 && idx < len(bl) && bl[idx].Foldable && !bl[idx].Collapsed {
					c.store.ToggleCollapse()
				}
			case "l", "right":
				idx := c.store.SelectedIndex()
				bl := c.store.Blocks()
				if idx >= 0 && idx < len(bl) && bl[idx].Foldable && bl[idx].Collapsed {
					c.store.ToggleCollapse()
				}
			case "e":
				c.store.ToggleCollapse()
			case "E":
				c.store.ToggleExpandAll()
			case "g":
				c.store.GotoTop()
			case "G":
				c.store.GotoBottom()
			case "ctrl+j":
				c.store.LineDown()
			case "pgdown", "ctrl+d":
				c.store.PageDown(msg.String() == "ctrl+d")
			case "pgup", "ctrl+u":
				c.store.PageUp(msg.String() == "ctrl+u")
			}
		}
	}

	if c.mode == ModeInsert && c.useTA {
		if km, ok := msg.(tea.KeyMsg); ok {
			if km.String() != "enter" && km.String() != "ctrl+c" {
				var cmd tea.Cmd
				c.ta, cmd = c.ta.Update(msg)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
		} else {
			var cmd tea.Cmd
			c.ta, cmd = c.ta.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}

	return c, tea.Batch(cmds...)
}

// AddSystemMessage uses typewriter when motion enabled.
func (c *ChatModel) AddSystemMessage(text string) {
	if theme.MotionEnabled() && strings.Count(text, "\n") > 2 {
		c.typewriterQ = strings.Split(text, "\n")
		c.typewriterOn = true
		return
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		c.store.AddSystem(line)
	}
}

func (c *ChatModel) Clear() {
	c.store.Clear()
	c.messages = nil
	c.streamFull = ""
	c.agentFull = ""
	c.streaming = false
	c.attachments = make(map[string]string)
	c.store.AddSystem("Chat cleared.")
}

func (c *ChatModel) LoadMessages(msgs []provider.Message) {
	c.messages = msgs
	c.store.Clear()
	c.store.AddSystem("Session resumed.")
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
			c.store.AddUser(m.Content)
		case provider.RoleAssistant:
			c.store.StartAssistant()
			c.store.AppendAssistantChunk(m.Content)
			c.store.SealAssistant()
		}
	}
	c.store.GotoBottom()
}

// ViewScrollback renders the block engine viewport.
func (c ChatModel) ViewScrollback() string {
	w := c.width
	if w < 10 {
		w = 10
	}
	t := theme.Current()
	var sb strings.Builder
	if c.streaming {
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		label := "thinking"
		if c.agentFull != "" {
			label = "working"
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(t.AccentRunning).Render(sp+" "+label))
		if !c.store.Following() {
			sb.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render("  ·  G follow"))
		}
		sb.WriteByte('\n')
		// reserve 1 line for status
		c.store.SetSize(w, max(1, c.height-1))
	} else {
		c.store.SetSize(w, c.height)
	}
	c.store.SetShowSelection(c.mode == ModeNormal)
	sb.WriteString(c.store.View())
	return lipgloss.NewStyle().Width(w).Height(c.height).Render(sb.String())
}

// ViewPrompt is the Grok-style bottom composer.
func (c ChatModel) ViewPrompt(focused bool) string {
	w := c.width
	if w < 10 {
		w = 10
	}
	t := theme.Current()
	c.ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(t.AccentUser).Bold(true)
	c.ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(t.TextMuted)
	c.ta.Prompt = "❯ "
	if !focused {
		c.ta.Blur()
	}

	var body strings.Builder
	if c.streaming && focused {
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		body.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render(sp+" agent running — Esc scrollback · G follow") + "\n")
	}
	body.WriteString(c.ta.View())
	if len(c.attachments) > 0 {
		var names []string
		for n := range c.attachments {
			names = append(names, "@"+n)
		}
		body.WriteString("\n" + lipgloss.NewStyle().Foreground(t.AccentUser).Render("📎 "+strings.Join(names, " ")))
	}
	if strings.HasPrefix(strings.TrimSpace(c.ta.Value()), "/") {
		body.WriteString("\n" + c.slashHints())
	}
	return theme.PromptFrame(w, focused).Render(body.String())
}

func (c ChatModel) slashHints() string {
	t := theme.Current()
	inp := strings.TrimSpace(c.ta.Value())
	var matches []string
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd, inp) {
			matches = append(matches, cmd)
		}
		if len(matches) >= 6 {
			break
		}
	}
	if len(matches) == 0 {
		return lipgloss.NewStyle().Foreground(t.TextMuted).Render("  no matching commands")
	}
	return lipgloss.NewStyle().Foreground(t.AccentTool).Render("  " + strings.Join(matches, "  "))
}

func (c ChatModel) View() string {
	return c.ViewScrollback()
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type SpinnerTickMsg struct{}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const systemPrompt = `You are CodeForge TUI, an AI pair-programming assistant by NanoMind (2026).
Answer in the user's language. Be clear and complete. Use markdown code fences for code.`

const agentSystemPrompt = `You are CodeForge TUI, an AI pair-programming agent by NanoMind (2026).

TOOLS:
- Discovery: codebase_search, grep_search, read_file, list_dir, research
- Edits: search_replace, apply_patch (preferred), write_file (new/full rewrite only)
- Design plan: write_plan, exit_plan_mode, enter_plan_mode
- Verify: diagnostics, run_command
- Docs: fetch_url
- GitHub: github tool (pr_*, issue_*, babysit, push, pull, …)
- MCP tools may appear as mcp_<server>_<tool>

SESSION MODES (user cycles with Shift+Tab):
- BUILD: file writes are STAGED for review
- DESIGN: only plan.md may be written (via write_plan). Explore, design, exit_plan_mode.
- YOLO: writes apply immediately

INSTRUCTIONS:
- Follow Project rules section if present
- Prefer codebase_search → read_file → search_replace
- After edits run diagnostics; for CI use github babysit_once
- In DESIGN mode: never edit project source; write_plan then exit_plan_mode
- Reply in the user's language; be concise`
