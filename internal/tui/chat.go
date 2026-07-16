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
	c.addLine(lineSystem, "CodeForge TUI v0.3.0  ·  NanoMind 2026  ·  Apache 2.0  ·  Neo-Forge")
	c.addLine(lineDivider, "")
	c.addLine(lineSystem, "i: chat  ·  /act <task>: agent  ·  Ctrl+K: palette  ·  @: file  ·  Shift+P: Plan/Act")
	c.addLine(lineSystem, "Default mode: PLAN (write_file menunggu review). Ketik ? untuk help.")
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
			System:    systemPrompt,
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
			System:    agentSystemPrompt,
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
	innerW := c.width - 6
	if innerW < 10 {
		innerW = 10
	}

	// Group consecutive assistant lines into markdown blocks
	var rendered []string
	var mdBuf strings.Builder
	flushMD := func() {
		if mdBuf.Len() == 0 {
			return
		}
		out := markdown.Render(mdBuf.String(), innerW)
		for _, ln := range strings.Split(out, "\n") {
			rendered = append(rendered, ln)
		}
		mdBuf.Reset()
	}

	for _, l := range c.lines {
		switch l.kind {
		case lineDivider:
			flushMD()
			rendered = append(rendered, "")
		case lineUser:
			flushMD()
			prefix := "▶ "
			wrapped := wordwrap.String(l.text, innerW-len(prefix))
			for i, wl := range strings.Split(wrapped, "\n") {
				if i == 0 {
					rendered = append(rendered, theme.StyleUser().Render(prefix+wl))
				} else {
					rendered = append(rendered, theme.StyleUser().Render("  "+wl))
				}
			}
		case lineAssistant:
			mdBuf.WriteString(l.text)
			mdBuf.WriteByte('\n')
		case lineSystem:
			flushMD()
			wrapped := wordwrap.String(l.text, innerW-2)
			for _, wl := range strings.Split(wrapped, "\n") {
				rendered = append(rendered, theme.StyleTextMuted().Render("  "+wl))
			}
		case lineToolCall:
			flushMD()
			// vertical timeline connector
			rendered = append(rendered,
				lipgloss.NewStyle().Foreground(t.AccentAgent).Render("  │ "+l.text))
		case lineToolResult:
			flushMD()
			color := t.Success
			if strings.HasPrefix(l.text, "✗") {
				color = t.Danger
			}
			rendered = append(rendered,
				lipgloss.NewStyle().Foreground(color).Render("  └ "+l.text))
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

func (c ChatModel) View() string {
	w := c.width
	if w < 10 {
		w = 10
	}
	t := theme.Current()
	innerW := w - 4
	if innerW < 4 {
		innerW = 4
	}

	var sb strings.Builder
	header := "Chat"
	if c.streaming {
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		if c.agentFull != "" {
			header = sp + " Agent"
		} else {
			header = sp + " Streaming"
		}
	}
	sb.WriteString(theme.StyleHeader().Render(header) + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", innerW)) + "\n")

	if c.ready {
		sb.WriteString(c.vp.View() + "\n")
	}

	sb.WriteString(lipgloss.NewStyle().Foreground(t.BorderDim).Render(strings.Repeat("─", innerW)) + "\n")

	switch {
	case c.streaming:
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		sb.WriteString(lipgloss.NewStyle().Foreground(t.TextMuted).Render(sp + " menunggu AI...") + "\n")
	case c.mode == ModeInsert:
		// style textarea
		c.ta.FocusedStyle.Prompt = lipgloss.NewStyle().Foreground(t.AccentAI).Bold(true)
		c.ta.BlurredStyle.Prompt = lipgloss.NewStyle().Foreground(t.TextMuted)
		sb.WriteString(c.ta.View() + "\n")
		if len(c.attachments) > 0 {
			var names []string
			for n := range c.attachments {
				names = append(names, "@"+n)
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(t.AccentUser).Render("  📎 "+strings.Join(names, " ")) + "\n")
		}
	default:
		hint := "(i) chat  (/) command  (Ctrl+K) palette  (?) help"
		sb.WriteString(lipgloss.NewStyle().Foreground(t.TextDisabled).Render(hint) + "\n")
	}

	return lipgloss.NewStyle().Width(w).Height(c.height).Render(sb.String())
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

const agentSystemPrompt = `Kamu adalah CodeForge TUI, asisten AI pair programming yang dibuat oleh NanoMind (2026).
Kamu memiliki akses ke tool filesystem: read_file, write_file, list_dir, grep_search, run_command.

INSTRUKSI:
- Selalu baca file sebelum mengeditnya
- Gunakan tool secara sistematis untuk menyelesaikan task
- Setelah edit file, jalankan go build atau test untuk verifikasi
- Jawab dalam Bahasa Indonesia
- Berikan penjelasan singkat tentang apa yang kamu lakukan
- write_file mungkin di-stage (Plan mode) — tetap panggil tool; user akan review`
