package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

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

type ChatModel struct {
	providerReg *provider.Registry
	toolReg     *tool.Registry
	gitRepo     *git.Repo
	workdir     string

	width  int
	height int
	input  string
	// lines is the raw display buffer (shown in the pane).
	lines     []string
	cursor    int
	streaming bool // true while stream OR agent loop is running
	mode      Mode

	// messages is the canonical conversation history sent to the LLM.
	messages []provider.Message

	// agentFull accumulates EventText chunks during an agent run so that
	// FinalizeAgentResponse can store a clean assistant message.
	agentFull string
}

func NewChatModel(provReg *provider.Registry, toolReg *tool.Registry, repo *git.Repo, workdir string) ChatModel {
	return ChatModel{
		providerReg: provReg,
		toolReg:     toolReg,
		gitRepo:     repo,
		workdir:     workdir,
		lines: []string{
			"CodeForge TUI v0.1.0-alpha",
			"Created by NanoMind - 2026",
			"",
			"  CHAT mode  — type 'i' then Enter to send a message",
			"  AGENT mode — /act <task>  (reads/writes files, runs tools)",
			"  TYPE /help for all commands",
			"",
		},
	}
}

func (c ChatModel) Init() tea.Cmd { return nil }

func (c *ChatModel) SetSize(w, h int) { c.width = w; c.height = h }
func (c *ChatModel) TypeText(s string) { c.input += s }
func (c *ChatModel) Backspace() {
	if len(c.input) > 0 {
		c.input = c.input[:len(c.input)-1]
	}
}
func (c *ChatModel) SetInput(s string) { c.input = s }

// ────────────────────────────────────────────────────────────
// Submit — streaming chat (no tool calls)
// ────────────────────────────────────────────────────────────

func (c *ChatModel) Submit() tea.Cmd {
	if c.input == "" || c.streaming {
		return nil
	}
	userMsg := c.input
	c.input = ""
	c.AddUserMessage(userMsg)
	c.streaming = true

	msgs := make([]provider.Message, len(c.messages))
	copy(msgs, c.messages)

	prov := c.providerReg
	return func() tea.Msg {
		p, err := prov.Current()
		if err != nil {
			return errMsg{err: fmt.Errorf("no provider: %w", err)}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer cancel()

		req := provider.CompletionRequest{
			Messages:  msgs,
			MaxTokens: 4096,
			System:    systemPrompt,
		}

		ch, err := p.Stream(ctx, req)
		if err != nil {
			return errMsg{err: err}
		}

		select {
		case t, ok := <-ch:
			if !ok {
				return StreamOpenedMsg{Ch: nil}
			}
			return StreamOpenedMsg{Ch: ch, FirstToken: t}
		case <-time.After(30 * time.Second):
			return errMsg{err: fmt.Errorf("stream timeout")}
		}
	}
}

// ────────────────────────────────────────────────────────────
// SubmitAgent — tool-calling agent loop
// ────────────────────────────────────────────────────────────

// SubmitAgent starts the agent loop for a task that may require tool calls
// (read_file, write_file, grep_search, run_command, etc.).
// It is triggered by the /act slash command.
func (c *ChatModel) SubmitAgent(task string) tea.Cmd {
	if strings.TrimSpace(task) == "" || c.streaming {
		return nil
	}
	c.AddUserMessage("🤖 [agent] " + task)
	c.streaming = true

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

		// Pull first event so the UI can start rendering immediately.
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

	// ── Stream (plain chat) ──────────────────────────────
	case StreamTickMsg:
		if msg.Text != "" {
			c.AppendStreamingText(msg.Text)
		}
		if msg.Done {
			full := c.extractLastBlock()
			c.FinalizeStreaming(full)
			if msg.InputTokens > 0 || msg.OutputTokens > 0 {
				c.lines = append(c.lines,
					fmt.Sprintf("  [tokens: %d in / %d out]", msg.InputTokens, msg.OutputTokens), "")
			}
		}
		if msg.Error != nil {
			c.lines = append(c.lines, fmt.Sprintf("  ERROR: %v", msg.Error), "")
			c.streaming = false
		}

	// ── Agent loop ───────────────────────────────────────
	case AgentEventMsg:
		ev := msg.Ev
		switch ev.Kind {
		case agent.EventText:
			c.AppendStreamingText(ev.Text)
			c.agentFull += ev.Text

		case agent.EventToolCall:
			c.lines = append(c.lines,
				fmt.Sprintf("  🔧 %s(%s)", ev.ToolName, truncate(ev.ToolInput, 60)))

		case agent.EventToolResult:
			icon := "  ✓"
			if !ev.ToolSuccess {
				icon = "  ✗"
			}
			c.lines = append(c.lines,
				fmt.Sprintf("%s %s: %s", icon, ev.ToolName, truncate(ev.ToolOutput, 80)))

		case agent.EventDone:
			c.FinalizeAgentResponse(c.agentFull)
			if ev.InputTokens > 0 || ev.OutputTokens > 0 {
				c.lines = append(c.lines,
					fmt.Sprintf("  [tokens: %d in / %d out]", ev.InputTokens, ev.OutputTokens), "")
			}

		case agent.EventError:
			c.lines = append(c.lines,
				fmt.Sprintf("  ⚠ agent error: %v", ev.Error), "")
			c.streaming = false
			c.agentFull = ""
		}

	// ── Key nav in pane ─────────────────────────────────
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			c.cursor++
		case "k", "up":
			if c.cursor > 0 {
				c.cursor--
			}
		case "g":
			c.cursor = 0
		case "G":
			c.cursor = len(c.lines)
		}
	}
	return c, nil
}

// ────────────────────────────────────────────────────────────
// Message helpers
// ────────────────────────────────────────────────────────────

func (c *ChatModel) AddUserMessage(text string) {
	c.messages = append(c.messages, provider.Message{Role: provider.RoleUser, Content: text})
	c.lines = append(c.lines, fmt.Sprintf("> %s", text), "")
}

func (c *ChatModel) AddSystemMessage(text string) {
	for _, line := range strings.Split(text, "\n") {
		c.lines = append(c.lines, "  "+line)
	}
	c.lines = append(c.lines, "")
}

func (c *ChatModel) AddAssistantMessage(text string) {
	c.messages = append(c.messages, provider.Message{Role: provider.RoleAssistant, Content: text})
}

func (c *ChatModel) AppendStreamingText(text string) {
	if len(c.lines) == 0 || strings.HasPrefix(c.lines[len(c.lines)-1], "> ") {
		c.lines = append(c.lines, "")
	}
	last := len(c.lines) - 1
	if c.lines[last] == "" {
		c.lines[last] = text
	} else {
		c.lines[last] += text
	}
}

func (c *ChatModel) FinalizeStreaming(fullText string) {
	c.streaming = false
	c.AddAssistantMessage(fullText)
	c.lines = append(c.lines, "")
}

func (c *ChatModel) FinalizeAgentResponse(fullText string) {
	c.streaming = false
	if fullText != "" {
		c.AddAssistantMessage(fullText)
	}
	c.lines = append(c.lines, "")
	c.agentFull = ""
}

func (c *ChatModel) Clear() {
	c.lines = []string{"Chat cleared.", ""}
	c.messages = nil
	c.agentFull = ""
	c.streaming = false
}

// extractLastBlock retrieves the last continuous non-empty, non-prefixed
// block of lines, which is the streamed assistant response.
func (c *ChatModel) extractLastBlock() string {
	for i := len(c.lines) - 1; i >= 0; i-- {
		line := c.lines[i]
		if line != "" && !strings.HasPrefix(line, "> ") && !strings.HasPrefix(line, "  ") {
			return line
		}
	}
	return ""
}

// ────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────

func (c ChatModel) View() string {
	var sb strings.Builder
	sb.WriteString("Chat\n")
	sb.WriteString(strings.Repeat("-", max(0, c.width-4)) + "\n")

	visH := c.height - 5
	if visH < 3 {
		visH = 3
	}
	start := 0
	if len(c.lines) > visH {
		start = len(c.lines) - visH
	}
	for i := start; i < len(c.lines); i++ {
		sb.WriteString(c.lines[i] + "\n")
	}

	sb.WriteString("\n" + strings.Repeat("-", max(0, c.width-4)) + "\n")
	inputLine := c.input
	switch {
	case c.streaming && c.agentFull != "":
		inputLine = "[agent running...]"
	case c.streaming:
		inputLine = "[streaming...]"
	case c.mode == ModeInsert:
		inputLine += "_"
	case inputLine == "":
		inputLine = "(i to type · /act <task> for agent mode)"
	}
	sb.WriteString("> " + inputLine + "\n")

	return lipgloss.NewStyle().
		Width(c.width).
		Height(c.height).
		Foreground(lipgloss.Color("#E2E8F0")).
		Render(sb.String())
}

// ────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
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

const systemPrompt = `You are CodeForge TUI, an AI pair programming assistant created by NanoMind.
Be concise and helpful. Answer in plain text unless the user asks for code.`

const agentSystemPrompt = `You are CodeForge TUI, an AI pair programming assistant created by NanoMind.
You have access to file system tools (read_file, write_file, list_dir, grep_search, run_command).
Use them systematically to complete the requested coding task.
Always read files before editing them. Run tests after making changes.
Be concise in explanations; show code changes rather than describing them.`
