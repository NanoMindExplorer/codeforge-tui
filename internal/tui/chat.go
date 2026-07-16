package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/reflow/wordwrap"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/git"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/ui/markdown"
)

// chatLine represents one logical line in the chat buffer.
type chatLine struct {
	text  string
	kind  lineKind
	rawMD bool // render via glamour when true
}

type lineKind int

const (
	lineUser lineKind = iota
	lineAssistant
	lineSystem
	lineToolCall
	lineToolResult
	lineDivider
)

type ChatModel struct {
	providerReg *provider.Registry
	toolReg     *tool.Registry
	gitRepo     *git.Repo
	workdir     string

	width  int
	height int

	vp       viewport.Model
	ta       textarea.Model
	ready    bool
	useTA    bool // true once sized

	lines    []chatLine
	atBottom bool

	streaming bool
	mode      Mode

	messages   []provider.Message
	agentFull  string
	streamFull string
	spinnerFrame int

	// typewriter for system messages
	typewriterQ    []string
	typewriterBuf  string
	typewriterOn   bool

	// input history (↑)
	history    []string
	historyIdx int

	// attached @files for next submit
	attachments map[string]string

	// project rules injected into system prompts
	rulesText string
}

func NewChatModel(provReg *provider.Registry, toolReg *tool.Registry, repo *git.Repo, workdir string) ChatModel {
	ta := textarea.New()
	ta.Placeholder = "ketik pesan, /command, atau @file…"
	ta.ShowLineNumbers = false
	ta.CharLimit = 32_000
	ta.SetHeight(3)
	ta.Prompt = "» "
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()

	c := ChatModel{
		providerReg: provReg,
		toolReg:     toolReg,
		gitRepo:     repo,
		workdir:     workdir,
		atBottom:    true,
		ta:          ta,
		attachments: make(map[string]string),
	}
	c.addLine(lineSystem, "CodeForge · Grok-style TUI  ·  type to chat  ·  / for commands  ·  ? help")
	c.addLine(lineDivider, "")
	c.addLine(lineSystem, "Tab focus · @ file · Ctrl+K palette · Shift+Tab Plan/Act · Ctrl+B panels")
	c.addLine(lineSystem, "Default write mode: PLAN (edits staged for review). Theme: GrokNight.")
	c.addLine(lineDivider, "")
	return c
}

func (c ChatModel) Init() tea.Cmd { return textarea.Blink }

func (c *ChatModel) SetSize(w, h int) {
	c.width = w
	c.height = h
	innerW := w - 4
	if innerW < 10 {
		innerW = 10
	}
	// header 2 + input 4 + pad
	vpH := h - 7
	if vpH < 3 {
		vpH = 3
	}
	if !c.ready {
		c.vp = viewport.New(innerW, vpH)
		c.ready = true
	} else {
		c.vp.Width = innerW
		c.vp.Height = vpH
	}
	c.ta.SetWidth(innerW)
	c.ta.SetHeight(3)
	c.useTA = true
	c.refreshViewport()
}

func (c *ChatModel) InputValue() string {
	if c.useTA {
		return c.ta.Value()
	}
	return ""
}

func (c *ChatModel) SetInput(s string) {
	c.ta.SetValue(s)
}

func (c *ChatModel) ClearInput() {
	c.ta.Reset()
}

func (c *ChatModel) FocusInput()  { c.ta.Focus() }
func (c *ChatModel) BlurInput()   { c.ta.Blur() }
func (c *ChatModel) InputFocused() bool { return c.ta.Focused() }

func (c *ChatModel) addLine(kind lineKind, text string) {
	c.lines = append(c.lines, chatLine{kind: kind, text: text, rawMD: kind == lineAssistant})
}

func (c *ChatModel) AttachFile(rel, content string) {
	if c.attachments == nil {
		c.attachments = make(map[string]string)
	}
	c.attachments[rel] = content
}

// SetRules installs project rules text for system prompt injection.
func (c *ChatModel) SetRules(text string) {
	c.rulesText = text
}

func (c *ChatModel) systemWithRules(base string) string {
	if c.rulesText == "" {
		return base
	}
	return rules.Inject(base, &rules.Bundle{Text: c.rulesText})
}

// ────────────────────────────────────────────────────────────
// Submit
// ────────────────────────────────────────────────────────────

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

	// Attach @file contents
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

	c.addLine(lineUser, userMsg)
	c.addLine(lineDivider, "")
	c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: fullContent})
	c.streaming = true
	c.streamFull = ""
	c.atBottom = true
	c.refreshViewport()

	msgs := make([]provider.Message, len(c.messages))
	copy(msgs, c.messages)
	prov := c.providerReg

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
		ch, err := p.Stream(context.Background(), req)
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

	// Attach files if any
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

	c.addLine(lineUser, "🤖 [agent] "+task)
	c.addLine(lineDivider, "")
	c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: task})
	c.streaming = true
	c.agentFull = ""
	c.atBottom = true
	c.refreshViewport()

	msgs := make([]provider.Message, len(c.messages))
	copy(msgs, c.messages)
	prov := c.providerReg
	toolReg := c.toolReg

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
		}
		ch := agent.Run(context.Background(), cfg, msgs)
		first, ok := <-ch
		if !ok {
			return AgentOpenedMsg{Ch: nil, First: agent.Event{Kind: agent.EventDone}}
		}
		return AgentOpenedMsg{Ch: ch, First: first}
	}
}

// ────────────────────────────────────────────────────────────
// Update
// ────────────────────────────────────────────────────────────

func (c ChatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SpinnerTickMsg:
		if c.streaming {
			c.spinnerFrame = (c.spinnerFrame + 1) % len(spinnerFrames)
		}
		// typewriter reveal
		if c.typewriterOn && len(c.typewriterQ) > 0 {
			// reveal up to 2 lines per tick, max ~80ms total feel
			for i := 0; i < 2 && len(c.typewriterQ) > 0; i++ {
				c.addLine(lineSystem, c.typewriterQ[0])
				c.typewriterQ = c.typewriterQ[1:]
			}
			if len(c.typewriterQ) == 0 {
				c.addLine(lineDivider, "")
				c.typewriterOn = false
			}
			c.atBottom = true
			c.refreshViewport()
		}

	case StreamTickMsg:
		if msg.Error != nil {
			c.addLine(lineSystem, "⚠ Error: "+msg.Error.Error())
			c.addLine(lineDivider, "")
			c.streaming = false
			c.streamFull = ""
			c.refreshViewport()
			break
		}
		if msg.Text != "" {
			c.appendStreamingChunk(msg.Text)
			c.streamFull += msg.Text
		}
		if msg.Done {
			if c.streamFull != "" {
				c.messages = append(c.messages, provider.Message{
					Role: provider.RoleAssistant, Content: c.streamFull,
				})
			}
			c.addLine(lineDivider, "")
			if msg.InputTokens > 0 || msg.OutputTokens > 0 {
				c.addLine(lineSystem, fmt.Sprintf("tokens: %d in / %d out", msg.InputTokens, msg.OutputTokens))
				c.addLine(lineDivider, "")
			}
			c.streaming = false
			c.streamFull = ""
		}
		c.atBottom = true
		c.refreshViewport()

	case AgentEventMsg:
		ev := msg.Ev
		switch ev.Kind {
		case agent.EventText:
			c.appendStreamingChunk(ev.Text)
			c.agentFull += ev.Text
		case agent.EventToolCall:
			c.sealStreamingLine()
			icon := theme.ToolIcon(ev.ToolName)
			c.addLine(lineToolCall, fmt.Sprintf("%s %s  %s", icon, ev.ToolName, truncate(ev.ToolInput, 55)))
		case agent.EventToolProgress:
			// Live tool output chunks (babysit, long shell, …)
			txt := strings.TrimSpace(ev.Progress)
			if txt != "" {
				c.addLine(lineSystem, "⋯ "+truncate(txt, 100))
			}
		case agent.EventToolResult:
			icon := "✓"
			if !ev.ToolSuccess {
				icon = "✗"
			}
			c.addLine(lineToolResult, fmt.Sprintf("%s %s: %s", icon, ev.ToolName, truncate(ev.ToolOutput, 70)))
		case agent.EventDone:
			c.sealStreamingLine()
			if c.agentFull != "" {
				c.messages = append(c.messages, provider.Message{
					Role: provider.RoleAssistant, Content: c.agentFull,
				})
			}
			c.addLine(lineDivider, "")
			if ev.InputTokens > 0 || ev.OutputTokens > 0 {
				c.addLine(lineSystem, fmt.Sprintf("tokens: %d in / %d out", ev.InputTokens, ev.OutputTokens))
				c.addLine(lineDivider, "")
			}
			c.streaming = false
			c.agentFull = ""
		case agent.EventError:
			c.sealStreamingLine()
			c.addLine(lineSystem, "⚠ agent: "+ev.Error.Error())
			c.addLine(lineDivider, "")
			c.streaming = false
			c.agentFull = ""
		}
		c.atBottom = true
		c.refreshViewport()

	case tea.KeyMsg:
		// history navigation when not streaming and textarea focused
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
		// viewport scroll in normal mode
		if c.mode == ModeNormal {
			var cmd tea.Cmd
			c.vp, cmd = c.vp.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			switch msg.String() {
			case "j", "down":
				c.vp.LineDown(1)
				c.atBottom = false
			case "k", "up":
				c.vp.LineUp(1)
				c.atBottom = false
			case "g":
				c.vp.GotoTop()
				c.atBottom = false
			case "G":
				c.vp.GotoBottom()
				c.atBottom = true
			case "pgdown", "ctrl+d":
				c.vp.HalfViewDown()
			case "pgup", "ctrl+u":
				c.vp.HalfViewUp()
			}
		}
	}

	// Forward to textarea in insert mode
	if c.mode == ModeInsert && c.useTA {
		if km, ok := msg.(tea.KeyMsg); ok {
			// Don't let enter submit through textarea — model.go handles enter
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

func (c *ChatModel) appendStreamingChunk(text string) {
	if len(c.lines) == 0 || c.lines[len(c.lines)-1].kind != lineAssistant {
		c.lines = append(c.lines, chatLine{kind: lineAssistant, text: "", rawMD: true})
	}
	for _, ch := range text {
		if ch == '\n' {
			c.lines = append(c.lines, chatLine{kind: lineAssistant, text: "", rawMD: true})
		} else {
			last := &c.lines[len(c.lines)-1]
			last.text += string(ch)
		}
	}
}

func (c *ChatModel) sealStreamingLine() {
	if len(c.lines) > 0 && c.lines[len(c.lines)-1].kind == lineAssistant {
		if c.lines[len(c.lines)-1].text == "" {
			c.lines = c.lines[:len(c.lines)-1]
		}
	}
}

// AddSystemMessage uses typewriter when motion enabled.
func (c *ChatModel) AddSystemMessage(text string) {
	if theme.MotionEnabled() && strings.Count(text, "\n") > 2 {
		c.typewriterQ = strings.Split(text, "\n")
		c.typewriterOn = true
		return
	}
	for _, line := range strings.Split(text, "\n") {
		c.addLine(lineSystem, line)
	}
	c.addLine(lineDivider, "")
	c.atBottom = true
	c.refreshViewport()
}

func (c *ChatModel) Clear() {
	c.lines = nil
	c.messages = nil
	c.streamFull = ""
	c.agentFull = ""
	c.streaming = false
	c.atBottom = true
	c.attachments = make(map[string]string)
	c.addLine(lineSystem, "Chat cleared.")
	c.addLine(lineDivider, "")
	c.refreshViewport()
}

func (c *ChatModel) LoadMessages(msgs []provider.Message) {
	c.messages = msgs
	c.lines = nil
	c.addLine(lineSystem, "Session resumed.")
	c.addLine(lineDivider, "")
	for _, m := range msgs {
		switch m.Role {
		case provider.RoleUser:
			c.addLine(lineUser, m.Content)
		case provider.RoleAssistant:
			// store as multi-line assistant for markdown
			for _, ln := range strings.Split(m.Content, "\n") {
				c.addLine(lineAssistant, ln)
			}
		}
		c.addLine(lineDivider, "")
	}
	c.atBottom = true
	c.refreshViewport()
}

func (c *ChatModel) refreshViewport() {
	if !c.ready {
		return
	}
	content := c.renderLines()
	c.vp.SetContent(content)
	if c.atBottom {
		c.vp.GotoBottom()
	}
}

func (c *ChatModel) renderLines() string {
	t := theme.Current()
	// Grok-style: content width with left accent + padding
	hPad := 2
	if theme.CompactMode() {
		hPad = 1
	}
	innerW := c.width - 4 - hPad
	if innerW < 10 {
		innerW = 10
	}

	bar := func(color lipgloss.Color) string {
		return theme.BlockPrefix(color)
	}

	var rendered []string
	var mdBuf strings.Builder
	flushMD := func() {
		if mdBuf.Len() == 0 {
			return
		}
		out := markdown.Render(mdBuf.String(), innerW-2)
		pfx := bar(t.AccentAssistant)
		for _, ln := range strings.Split(out, "\n") {
			rendered = append(rendered, pfx+ln)
		}
		mdBuf.Reset()
		rendered = append(rendered, "")
	}

	for _, l := range c.lines {
		switch l.kind {
		case lineDivider:
			flushMD()
		case lineUser:
			flushMD()
			// Grok user block: magenta accent + light bg band
			pfx := bar(t.AccentUser)
			label := lipgloss.NewStyle().Foreground(t.AccentUser).Bold(true).Render("you")
			rendered = append(rendered, pfx+label)
			wrapped := wordwrap.String(l.text, innerW-2)
			bg := lipgloss.NewStyle().Foreground(t.TextPrimary).Background(t.BgLight)
			for _, wl := range strings.Split(wrapped, "\n") {
				rendered = append(rendered, pfx+bg.Width(innerW-2).Render(wl))
			}
			rendered = append(rendered, "")
		case lineAssistant:
			mdBuf.WriteString(l.text)
			mdBuf.WriteByte('\n')
		case lineSystem:
			flushMD()
			pfx := bar(t.AccentSystem)
			wrapped := wordwrap.String(l.text, innerW-2)
			for _, wl := range strings.Split(wrapped, "\n") {
				rendered = append(rendered, pfx+theme.StyleTextMuted().Render(wl))
			}
		case lineToolCall:
			flushMD()
			// Grok tool block: diamond bullet + tool accent
			pfx := bar(t.AccentTool)
			body := lipgloss.NewStyle().Foreground(t.AccentTool).Render(l.text)
			rendered = append(rendered, pfx+body)
		case lineToolResult:
			flushMD()
			color := t.Success
			if strings.HasPrefix(l.text, "✗") {
				color = t.Danger
			}
			pfx := bar(t.AccentTool)
			body := lipgloss.NewStyle().Foreground(color).Faint(true).Render("  "+l.text)
			rendered = append(rendered, pfx+body)
		}
	}
	flushMD()
	return strings.Join(rendered, "\n")
}

// ────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type SpinnerTickMsg struct{}

// ViewScrollback is the Grok-style conversation pane (no composer).
func (c ChatModel) ViewScrollback() string {
	w := c.width
	if w < 10 {
		w = 10
	}
	t := theme.Current()
	var sb strings.Builder
	// Subtle top status when streaming
	if c.streaming {
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		label := "thinking"
		if c.agentFull != "" || (len(c.lines) > 0 && c.lines[len(c.lines)-1].kind == lineToolCall) {
			label = "working"
		}
		sb.WriteString(lipgloss.NewStyle().Foreground(t.AccentRunning).Render(sp+" "+label) + "\n")
	}
	if c.ready {
		sb.WriteString(c.vp.View())
	}
	return lipgloss.NewStyle().Width(w).Height(c.height).Render(sb.String())
}

// ViewPrompt is the Grok-style bottom composer.
func (c ChatModel) ViewPrompt(focused bool) string {
	w := c.width
	if w < 10 {
		w = 10
	}
	t := theme.Current()
	// Grok prompt prefix
	c.ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(t.AccentUser).Bold(true)
	c.ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(t.TextMuted)
	c.ta.Prompt = "❯ "
	if !focused {
		c.ta.Blur()
	}

	var body strings.Builder
	if c.streaming && focused {
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		body.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render(sp+" agent running — type to queue · Esc scrollback") + "\n")
	}
	body.WriteString(c.ta.View())
	if len(c.attachments) > 0 {
		var names []string
		for n := range c.attachments {
			names = append(names, "@"+n)
		}
		body.WriteString("\n" + lipgloss.NewStyle().Foreground(t.AccentUser).Render("📎 "+strings.Join(names, " ")))
	}
	// Slash hint when typing /
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

// View keeps backward compatibility for pane layouts.
func (c ChatModel) View() string {
	return c.ViewScrollback()
}

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

const systemPrompt = `Kamu adalah CodeForge TUI, asisten AI pair programming yang dibuat oleh NanoMind (2026).
Jawab dalam Bahasa Indonesia kecuali diminta lain.
Berikan jawaban yang jelas dan lengkap. Jangan potong penjelasan di tengah.
Untuk kode, gunakan blok markdown code.`

const agentSystemPrompt = `You are CodeForge TUI, an AI pair-programming agent by NanoMind (2026).

TOOLS:
- Discovery: codebase_search (index), grep_search, read_file, list_dir, research (read-only sub-agent)
- Edits: search_replace, apply_patch (preferred), write_file (new/full rewrite only)
- Verify: diagnostics (go build/vet/test or custom), run_command
- Docs: fetch_url (public https only; secrets redacted)
- GitHub: github tool (pr_*, issue_*, babysit, push, pull, …)
- MCP tools may appear as mcp_<server>_<tool>

INSTRUCTIONS:
- Follow Project rules section if present
- Never request or echo secrets; sensitive files are redacted automatically
- Prefer codebase_search → read_file → search_replace over blind rewrites
- After edits run diagnostics; for CI use github babysit_once / babysit
- Ship: commit → push → pr_create → babysit → fix → push
- Reply in the user's language; be concise; return URLs for PRs/issues`
