package tui

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/git"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/tool"
)

// ────────────────────────────────────────────────────────────
// ChatModel
// ────────────────────────────────────────────────────────────

// chatLine represents one logical line in the chat buffer.
type chatLine struct {
	text  string
	kind  lineKind // user | assistant | system | toolcall | toolresult | divider
}

type lineKind int

const (
	lineUser       lineKind = iota
	lineAssistant           // streamed/complete assistant text
	lineSystem              // info messages
	lineToolCall            // 🔧 tool invocation
	lineToolResult          // ✓/✗ tool output
	lineDivider             // blank separator
)

type ChatModel struct {
	providerReg *provider.Registry
	toolReg     *tool.Registry
	gitRepo     *git.Repo
	workdir     string

	width  int
	height int
	input  string

	lines     []chatLine
	scroll    int  // how many lines from top are hidden (scrolled past)
	atBottom  bool // should auto-scroll to newest content

	streaming bool
	mode      Mode

	// conversation history sent to the LLM
	messages []provider.Message

	// accumulates EventText during agent run for history storage
	agentFull     string
	// streamFull accumulates plain stream text
	streamFull    string
	// spinner frame
	spinnerFrame  int
}

func NewChatModel(provReg *provider.Registry, toolReg *tool.Registry, repo *git.Repo, workdir string) ChatModel {
	c := ChatModel{
		providerReg: provReg,
		toolReg:     toolReg,
		gitRepo:     repo,
		workdir:     workdir,
		atBottom:    true,
	}
	c.addLine(lineSystem, "CodeForge TUI v0.1.0-alpha  ·  NanoMind 2026  ·  Apache 2.0")
	c.addLine(lineDivider, "")
	c.addLine(lineSystem, "Ketik 'i' lalu Enter untuk chat streaming")
	c.addLine(lineSystem, "Ketik '/act <task>' untuk agent mode (baca/tulis file, jalankan command)")
	c.addLine(lineSystem, "Ketik '?' untuk help lengkap")
	c.addLine(lineDivider, "")
	return c
}

func (c ChatModel) Init() tea.Cmd { return nil }

func (c *ChatModel) SetSize(w, h int) {
	c.width = w
	c.height = h
}

func (c *ChatModel) TypeText(s string) { c.input += s }
func (c *ChatModel) Backspace() {
	if len(c.input) == 0 {
		return
	}
	// UTF-8 safe backspace
	_, size := utf8.DecodeLastRuneInString(c.input)
	c.input = c.input[:len(c.input)-size]
}
func (c *ChatModel) SetInput(s string) { c.input = s }

func (c *ChatModel) addLine(kind lineKind, text string) {
	c.lines = append(c.lines, chatLine{kind: kind, text: text})
}

// ────────────────────────────────────────────────────────────
// Submit — streaming chat (no tool calls)
// BUG FIX: context.Background() instead of WithTimeout+defer cancel
// which was killing the HTTP stream after the first token.
// ────────────────────────────────────────────────────────────

func (c *ChatModel) Submit() tea.Cmd {
	if c.input == "" || c.streaming {
		return nil
	}
	userMsg := strings.TrimSpace(c.input)
	if userMsg == "" {
		return nil
	}
	c.input = ""

	// BUG FIX #2: intercept slash commands typed in INSERT mode
	if strings.HasPrefix(userMsg, "/") {
		return nil // caller (model.go) handles slash routing via SubmitInput
	}

	c.addLine(lineUser, userMsg)
	c.addLine(lineDivider, "")
	c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: userMsg})
	c.streaming = true
	c.streamFull = ""
	c.atBottom = true

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

		// BUG FIX #1: use context.Background() — a WithTimeout + defer cancel()
		// cancels the HTTP request the moment this tea.Cmd function returns
		// (after only 1-3 tokens). Background() lets the stream goroutine
		// live until it naturally closes the channel.
		ch, err := p.Stream(context.Background(), req)
		if err != nil {
			return errMsg{err: err}
		}

		// Pull first token to confirm stream opened
		first, ok := <-ch
		if !ok {
			return StreamOpenedMsg{Ch: nil}
		}
		return StreamOpenedMsg{Ch: ch, FirstToken: first}
	}
}

// SubmitInput is called by model.go on Enter. It routes slash commands
// to the model's executeSlashCommand and regular text to Submit.
func (c *ChatModel) SubmitInput() (tea.Cmd, bool) {
	inp := strings.TrimSpace(c.input)
	if inp == "" {
		return nil, false
	}
	if strings.HasPrefix(inp, "/") {
		// Return the raw input; model.go will route it
		return nil, true // true = is slash command
	}
	return c.Submit(), false
}

// ────────────────────────────────────────────────────────────
// SubmitAgent — tool-calling agent loop
// ────────────────────────────────────────────────────────────

func (c *ChatModel) SubmitAgent(task string) tea.Cmd {
	task = strings.TrimSpace(task)
	if task == "" || c.streaming {
		return nil
	}

	c.addLine(lineUser, "🤖 [agent] "+task)
	c.addLine(lineDivider, "")
	c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: task})
	c.streaming = true
	c.agentFull = ""
	c.atBottom = true

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
	switch msg := msg.(type) {

	case SpinnerTickMsg:
		if c.streaming {
			c.spinnerFrame = (c.spinnerFrame + 1) % len(spinnerFrames)
		}

	// ── Plain streaming chat ─────────────────────────────
	case StreamTickMsg:
		if msg.Error != nil {
			c.addLine(lineSystem, "⚠ Error: "+msg.Error.Error())
			c.addLine(lineDivider, "")
			c.streaming = false
			c.streamFull = ""
			break
		}
		if msg.Text != "" {
			c.appendStreamingChunk(msg.Text)
			c.streamFull += msg.Text
		}
		if msg.Done {
			if c.streamFull != "" {
				c.messages = append(c.messages, provider.Message{
					Role:    provider.RoleAssistant,
					Content: c.streamFull,
				})
			}
			c.addLine(lineDivider, "")
			if msg.InputTokens > 0 || msg.OutputTokens > 0 {
				c.addLine(lineSystem, fmt.Sprintf(
					"tokens: %d in / %d out", msg.InputTokens, msg.OutputTokens))
				c.addLine(lineDivider, "")
			}
			c.streaming = false
			c.streamFull = ""
		}
		c.atBottom = true

	// ── Agent loop events ────────────────────────────────
	case AgentEventMsg:
		ev := msg.Ev
		switch ev.Kind {

		case agent.EventText:
			c.appendStreamingChunk(ev.Text)
			c.agentFull += ev.Text

		case agent.EventToolCall:
			// Finish any pending streamed text first
			c.sealStreamingLine()
			c.addLine(lineToolCall,
				fmt.Sprintf("🔧 %s  %s", ev.ToolName, truncate(ev.ToolInput, 55)))

		case agent.EventToolResult:
			icon := "✓"
			if !ev.ToolSuccess {
				icon = "✗"
			}
			c.addLine(lineToolResult,
				fmt.Sprintf("%s %s: %s", icon, ev.ToolName, truncate(ev.ToolOutput, 70)))

		case agent.EventDone:
			c.sealStreamingLine()
			if c.agentFull != "" {
				c.messages = append(c.messages, provider.Message{
					Role:    provider.RoleAssistant,
					Content: c.agentFull,
				})
			}
			c.addLine(lineDivider, "")
			if ev.InputTokens > 0 || ev.OutputTokens > 0 {
				c.addLine(lineSystem, fmt.Sprintf(
					"tokens: %d in / %d out", ev.InputTokens, ev.OutputTokens))
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

	// ── Scroll ───────────────────────────────────────────
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			c.scroll++
			c.atBottom = false
		case "k", "up":
			if c.scroll > 0 {
				c.scroll--
			}
			c.atBottom = false
		case "g":
			c.scroll = 0
			c.atBottom = false
		case "G":
			c.atBottom = true
		}
	}
	return c, nil
}

// ────────────────────────────────────────────────────────────
// Streaming buffer helpers
// ────────────────────────────────────────────────────────────

// appendStreamingChunk appends text to the current assistant streaming line(s).
// It handles embedded newlines correctly so multi-paragraph responses render
// as separate lines rather than one giant concatenated line.
func (c *ChatModel) appendStreamingChunk(text string) {
	// Ensure there's an assistant line at the bottom to write into
	if len(c.lines) == 0 || c.lines[len(c.lines)-1].kind != lineAssistant {
		c.lines = append(c.lines, chatLine{kind: lineAssistant, text: ""})
	}

	for _, ch := range text {
		if ch == '\n' {
			// Start a new assistant line
			c.lines = append(c.lines, chatLine{kind: lineAssistant, text: ""})
		} else {
			last := &c.lines[len(c.lines)-1]
			last.text += string(ch)
		}
	}
}

// sealStreamingLine ensures the last assistant line is "closed" before
// adding a tool call / result separator.
func (c *ChatModel) sealStreamingLine() {
	if len(c.lines) > 0 && c.lines[len(c.lines)-1].kind == lineAssistant {
		if c.lines[len(c.lines)-1].text == "" {
			c.lines = c.lines[:len(c.lines)-1] // remove empty trailing line
		}
	}
}

// ────────────────────────────────────────────────────────────
// AddSystemMessage (called by model.go for /help, /status etc.)
// ────────────────────────────────────────────────────────────

func (c *ChatModel) AddSystemMessage(text string) {
	for _, line := range strings.Split(text, "\n") {
		c.addLine(lineSystem, line)
	}
	c.addLine(lineDivider, "")
	c.atBottom = true
}

func (c *ChatModel) Clear() {
	c.lines = nil
	c.messages = nil
	c.streamFull = ""
	c.agentFull = ""
	c.streaming = false
	c.scroll = 0
	c.atBottom = true
	c.addLine(lineSystem, "Chat cleared.")
	c.addLine(lineDivider, "")
}

// ────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// SpinnerTickMsg is sent by the spinner ticker in model.go
type SpinnerTickMsg struct{}

func (c ChatModel) View() string {
	w := c.width
	if w < 10 {
		w = 10
	}
	innerW := w - 6 // account for border + padding

	// ── render each line into display rows (with word-wrap) ──
	var rendered []string
	for _, l := range c.lines {
		switch l.kind {
		case lineDivider:
			rendered = append(rendered, "")
		case lineUser:
			prefix := "▶ "
			wrapped := wordWrap(l.text, innerW-len(prefix))
			for i, wl := range wrapped {
				if i == 0 {
					rendered = append(rendered,
						lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true).Render(prefix+wl))
				} else {
					rendered = append(rendered,
						lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Render("  "+wl))
				}
			}
		case lineAssistant:
			wrapped := wordWrap(l.text, innerW)
			for _, wl := range wrapped {
				rendered = append(rendered,
					lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0")).Render(wl))
			}
		case lineSystem:
			wrapped := wordWrap(l.text, innerW-2)
			for _, wl := range wrapped {
				rendered = append(rendered,
					lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Italic(true).Render("  "+wl))
			}
		case lineToolCall:
			rendered = append(rendered,
				lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Render("  "+l.text))
		case lineToolResult:
			color := lipgloss.Color("#10B981")
			if strings.HasPrefix(l.text, "✗") {
				color = lipgloss.Color("#EF4444")
			}
			rendered = append(rendered,
				lipgloss.NewStyle().Foreground(color).Render("  "+l.text))
		}
	}

	// ── compute visible window ───────────────────────────
	// Reserve: header(2) + input area(3) = 5 lines
	visH := c.height - 5
	if visH < 3 {
		visH = 3
	}

	totalLines := len(rendered)
	if c.atBottom || c.scroll > totalLines-visH {
		if totalLines > visH {
			c.scroll = totalLines - visH
		} else {
			c.scroll = 0
		}
	}
	if c.scroll < 0 {
		c.scroll = 0
	}

	var sb strings.Builder

	// Header
	header := "Chat"
	if c.streaming {
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		if c.agentFull != "" || (len(c.lines) > 0 && c.lines[len(c.lines)-1].kind == lineToolCall) {
			header = sp + " Agent"
		} else {
			header = sp + " Streaming"
		}
	}
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#06B6D4")).Render(header) + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#334155")).Render(strings.Repeat("─", max(0, innerW))) + "\n")

	// Visible lines
	end := c.scroll + visH
	if end > totalLines {
		end = totalLines
	}
	for i := c.scroll; i < end; i++ {
		sb.WriteString(rendered[i] + "\n")
	}
	// Pad remaining space
	for i := end - c.scroll; i < visH; i++ {
		sb.WriteString("\n")
	}

	// Input area
	sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#334155")).Render(strings.Repeat("─", max(0, innerW))) + "\n")

	var inputDisplay string
	switch {
	case c.streaming:
		sp := spinnerFrames[c.spinnerFrame%len(spinnerFrames)]
		inputDisplay = lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render(sp + " menunggu AI...")
	case c.mode == ModeInsert:
		display := c.input
		// Show hint if input starts with / to indicate slash command mode
		prefix := lipgloss.NewStyle().Foreground(lipgloss.Color("#06B6D4")).Bold(true).Render("» ")
		cursor := lipgloss.NewStyle().Background(lipgloss.Color("#06B6D4")).Foreground(lipgloss.Color("#000000")).Render(" ")
		inputDisplay = prefix + display + cursor
	default:
		hint := "(i) ketik pesan  (/) slash command  (?) help"
		inputDisplay = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render(hint)
	}
	sb.WriteString(inputDisplay + "\n")

	// Scroll indicator
	if totalLines > visH {
		scrollPct := 0
		if totalLines-visH > 0 {
			scrollPct = c.scroll * 100 / (totalLines - visH)
		}
		scrollInfo := fmt.Sprintf("↑↓ scroll  %d%%  [j/k/g/G]", scrollPct)
		sb.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("#334155")).Render(scrollInfo) + "\n")
	}

	return lipgloss.NewStyle().
		Width(w).
		Height(c.height).
		Render(sb.String())
}

// ────────────────────────────────────────────────────────────
// Word wrap helper
// ────────────────────────────────────────────────────────────

func wordWrap(text string, width int) []string {
	if width <= 0 {
		width = 40
	}
	if text == "" {
		return []string{""}
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := ""
		for _, word := range words {
			if line == "" {
				line = word
			} else if len(line)+1+len(word) <= width {
				line += " " + word
			} else {
				result = append(result, line)
				line = word
			}
		}
		if line != "" {
			result = append(result, line)
		}
	}
	return result
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	// strip newlines for single-line display
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

// ────────────────────────────────────────────────────────────
// System prompts
// ────────────────────────────────────────────────────────────

const systemPrompt = `Kamu adalah CodeForge TUI, asisten AI pair programming yang dibuat oleh NanoMind (2026).
Jawab dalam Bahasa Indonesia kecuali diminta lain.
Berikan jawaban yang jelas dan lengkap. Jangan potong penjelasan di tengah.
Untuk kode, gunakan blok ` + "`" + `code` + "`" + `.`

const agentSystemPrompt = `Kamu adalah CodeForge TUI, asisten AI pair programming yang dibuat oleh NanoMind (2026).
Kamu memiliki akses ke tool filesystem: read_file, write_file, list_dir, grep_search, run_command.

INSTRUKSI:
- Selalu baca file sebelum mengeditnya
- Gunakan tool secara sistematis untuk menyelesaikan task
- Setelah edit file, jalankan go build atau test untuk verifikasi
- Jawab dalam Bahasa Indonesia
- Berikan penjelasan singkat tentang apa yang kamu lakukan`
