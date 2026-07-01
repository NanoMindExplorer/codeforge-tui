package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/git"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/tool"
)

// ────────────────────────────────────────────────────────────
// Model
// ────────────────────────────────────────────────────────────

type Model struct {
	cfg         *config.Config
	providerReg *provider.Registry
	toolReg     *tool.Registry
	gitRepo     *git.Repo
	workdir     string

	width      int
	height     int
	activePane Pane
	mode       Mode

	chat    ChatModel
	diff    DiffModel
	context ContextModel
	status  StatusBarModel
	command CommandModel

	// Stream (plain chat)
	streamCh     <-chan provider.StreamToken
	streamInTok  int
	streamOutTok int

	// Agent loop
	agentCh <-chan agent.Event

	quitting    bool
	err         error
	startTime   time.Time
	totalCost   float64
	totalTokens int
}

type Pane int

const (
	PaneChat Pane = iota
	PaneDiff
	PaneContext
)

type Mode int

const (
	ModeNormal Mode = iota
	ModeInsert
	ModeCommand
)

// ────────────────────────────────────────────────────────────
// Constructor
// ────────────────────────────────────────────────────────────

func New(cfg *config.Config, provReg *provider.Registry, toolReg *tool.Registry, repo *git.Repo, workdir string) Model {
	return Model{
		cfg:         cfg,
		providerReg: provReg,
		toolReg:     toolReg,
		gitRepo:     repo,
		workdir:     workdir,
		activePane:  PaneChat,
		mode:        ModeNormal,
		startTime:   time.Now(),
		chat:        NewChatModel(provReg, toolReg, repo, workdir),
		diff:        NewDiffModel(),
		context:     NewContextModel(workdir),
		status:      NewStatusBarModel(),
		command:     NewCommandModel(),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.chat.Init(), m.context.Init())
}

// ────────────────────────────────────────────────────────────
// Update
// ────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ── Window resize ────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		mainH := msg.Height - 2
		if mainH < 5 {
			mainH = 5
		}
		chatW := msg.Width * 50 / 100
		diffW := msg.Width * 30 / 100
		ctxW := msg.Width - chatW - diffW - 4
		if chatW < 20 {
			chatW = 20
		}
		if diffW < 10 {
			diffW = 10
		}
		if ctxW < 10 {
			ctxW = 10
		}
		m.chat.SetSize(chatW, mainH)
		m.diff.SetSize(diffW, mainH)
		m.context.SetSize(ctxW, mainH)
		m.status.SetSize(msg.Width)
		m.command.SetSize(msg.Width, mainH)

	// ── Key input ────────────────────────────────────────
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "ctrl+l":
			return m, nil
		}

		if m.mode == ModeCommand {
			newCmd, cmd := m.command.Update(msg)
			m.command = newCmd.(CommandModel)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			if m.command.Done {
				action := m.command.FinalValue
				m.command = NewCommandModel()
				m.mode = ModeNormal
				if action == "quit" || action == "q" {
					m.quitting = true
					return m, tea.Quit
				}
				if c := m.executeSlashCommand(action); c != nil {
					cmds = append(cmds, c)
				}
			}
			return m, tea.Batch(cmds...)
		}

		switch m.mode {
		case ModeNormal:
			switch msg.String() {
			case "i":
				m.mode = ModeInsert
				return m, nil
			case ":":
				m.mode = ModeCommand
				m.command.Activate()
				return m, nil
			case "/":
				m.mode = ModeCommand
				m.command.ActivateWithPrefix("/")
				return m, nil
			case "1":
				m.activePane = PaneChat
			case "2":
				m.activePane = PaneDiff
			case "3":
				m.activePane = PaneContext
			case "tab":
				m.activePane = (m.activePane + 1) % 3
			case "shift+tab":
				m.activePane = (m.activePane + 2) % 3
			case "q":
				m.quitting = true
				return m, tea.Quit
			case "?":
				m.chat.AddSystemMessage(helpText())
				return m, nil
			case "enter":
				if !m.chat.streaming {
					if c := m.chat.Submit(); c != nil {
						cmds = append(cmds, c)
					}
				}
				return m, tea.Batch(cmds...)
			case "esc":
				m.mode = ModeNormal
				return m, nil
			}

		case ModeInsert:
			keyStr := msg.String()
			switch keyStr {
			case "esc":
				m.mode = ModeNormal
				return m, nil
			case "enter":
				if !m.chat.streaming {
					if c := m.chat.Submit(); c != nil {
						cmds = append(cmds, c)
					}
				}
				return m, tea.Batch(cmds...)
			case "backspace":
				m.chat.Backspace()
				return m, nil
			case "tab":
				m.chat.TypeText("\t")
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				m.chat.TypeText(string(msg.Runes))
				return m, nil
			}
			if len(keyStr) == 1 {
				m.chat.TypeText(keyStr)
				return m, nil
			}
			return m, nil
		}

		// Forward key to active pane
		switch m.activePane {
		case PaneChat:
			nc, c := m.chat.Update(msg)
			m.chat = nc.(ChatModel)
			if c != nil {
				cmds = append(cmds, c)
			}
		case PaneDiff:
			nd, c := m.diff.Update(msg)
			m.diff = nd.(DiffModel)
			if c != nil {
				cmds = append(cmds, c)
			}
		case PaneContext:
			nctx, c := m.context.Update(msg)
			m.context = nctx.(ContextModel)
			if c != nil {
				cmds = append(cmds, c)
			}
		}

	// ── Stream (plain chat) ──────────────────────────────
	case StreamTickMsg:
		nc, c := m.chat.Update(msg)
		m.chat = nc.(ChatModel)
		if c != nil {
			cmds = append(cmds, c)
		}
		if msg.InputTokens > 0 || msg.OutputTokens > 0 {
			m.totalTokens += msg.InputTokens + msg.OutputTokens
			m.totalCost += calculateCost(msg.InputTokens, msg.OutputTokens, m.providerReg.CurrentName())
		}
		if !msg.Done && m.streamCh != nil {
			cmds = append(cmds, pumpStream(m.streamCh))
		}
		if msg.Done {
			m.streamCh = nil
		}

	case StreamOpenedMsg:
		m.streamCh = msg.Ch
		// Process the first token immediately
		firstTickMsg := StreamTickMsg{
			Text:         msg.FirstToken.Text,
			Done:         msg.FirstToken.Done,
			InputTokens:  msg.FirstToken.InputTokens,
			OutputTokens: msg.FirstToken.OutputTokens,
			Error:        msg.FirstToken.Error,
		}
		nc, c := m.chat.Update(firstTickMsg)
		m.chat = nc.(ChatModel)
		if c != nil {
			cmds = append(cmds, c)
		}
		if firstTickMsg.InputTokens > 0 || firstTickMsg.OutputTokens > 0 {
			m.totalTokens += firstTickMsg.InputTokens + firstTickMsg.OutputTokens
			m.totalCost += calculateCost(firstTickMsg.InputTokens, firstTickMsg.OutputTokens, m.providerReg.CurrentName())
		}
		if !msg.FirstToken.Done && m.streamCh != nil {
			cmds = append(cmds, pumpStream(m.streamCh))
		}
		if msg.FirstToken.Done {
			m.streamCh = nil
		}

	// ── Agent loop ───────────────────────────────────────
	case AgentOpenedMsg:
		m.agentCh = msg.Ch
		cmds = append(cmds, m.dispatchAgentEvent(msg.First)...)
		if msg.First.Kind != agent.EventDone && msg.First.Kind != agent.EventError && m.agentCh != nil {
			cmds = append(cmds, pumpAgent(m.agentCh))
		}

	case AgentEventMsg:
		cmds = append(cmds, m.dispatchAgentEvent(msg.Ev)...)
		if msg.Ev.Kind != agent.EventDone && msg.Ev.Kind != agent.EventError && m.agentCh != nil {
			cmds = append(cmds, pumpAgent(m.agentCh))
		} else if msg.Ev.Kind == agent.EventDone || msg.Ev.Kind == agent.EventError {
			m.agentCh = nil
		}

	// ── Errors ───────────────────────────────────────────
	case errMsg:
		m.chat.AddSystemMessage(fmt.Sprintf("Error: %v", msg.err))
		m.chat.streaming = false
	}

	// ── Status bar always updated ────────────────────────
	m.status.Mode = modeString(m.mode)
	m.status.Provider = m.providerReg.CurrentName()
	m.status.Tokens = m.totalTokens
	m.status.Cost = m.totalCost
	if m.gitRepo != nil {
		if branch, err := m.gitRepo.Branch(); err == nil {
			m.status.Branch = branch
		}
	}
	m.status.Workdir = m.workdir
	m.chat.mode = m.mode

	return m, tea.Batch(cmds...)
}

// dispatchAgentEvent routes one agent.Event to the chat model and, when a
// write_file produces a diff, also updates the Diff pane.
func (m *Model) dispatchAgentEvent(ev agent.Event) []tea.Cmd {
	var cmds []tea.Cmd

	nc, c := m.chat.Update(AgentEventMsg{Ev: ev})
	m.chat = nc.(ChatModel)
	if c != nil {
		cmds = append(cmds, c)
	}

	// Token accounting
	if ev.Kind == agent.EventDone {
		m.totalTokens += ev.InputTokens + ev.OutputTokens
		m.totalCost += calculateCost(ev.InputTokens, ev.OutputTokens, m.providerReg.CurrentName())
	}

	// Diff pane: show write_file diffs
	if ev.Kind == agent.EventToolResult && ev.ToolDiff != "" {
		nd, dc := m.diff.Update(DiffUpdateMsg{Content: ev.ToolDiff})
		m.diff = nd.(DiffModel)
		if dc != nil {
			cmds = append(cmds, dc)
		}
	}

	return cmds
}

// ────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.quitting {
		return "Goodbye!\n"
	}

	topBar := m.status.ViewTop()
	chatStyle := paneStyle(m.activePane == PaneChat)
	diffStyle := paneStyle(m.activePane == PaneDiff)
	ctxStyle := paneStyle(m.activePane == PaneContext)

	mainRow := lipgloss.JoinHorizontal(lipgloss.Top,
		chatStyle.Render(m.chat.View()),
		diffStyle.Render(m.diff.View()),
		ctxStyle.Render(m.context.View()),
	)
	bottomBar := m.status.ViewBottom()

	if m.mode == ModeCommand {
		return lipgloss.JoinVertical(lipgloss.Left,
			topBar, mainRow, m.command.View(), bottomBar)
	}
	return lipgloss.JoinVertical(lipgloss.Left, topBar, mainRow, bottomBar)
}

// ────────────────────────────────────────────────────────────
// Slash commands
// ────────────────────────────────────────────────────────────

func (m *Model) executeSlashCommand(input string) tea.Cmd {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	if strings.HasPrefix(input, "/") {
		parts := strings.Fields(input[1:])
		if len(parts) == 0 {
			return nil
		}
		cmd := parts[0]
		args := parts[1:]

		switch cmd {
		case "help", "h":
			m.chat.AddSystemMessage(helpText())

		case "about", "a":
			m.chat.AddSystemMessage(aboutText())

		case "version", "v":
			m.chat.AddSystemMessage(versionText())

		case "provider", "p":
			if len(args) == 0 {
				names := m.providerReg.List()
				current := m.providerReg.CurrentName()
				var sb strings.Builder
				sb.WriteString("Available providers:\n")
				for _, name := range names {
					marker := "  "
					if name == current {
						marker = "* "
					}
					sb.WriteString(fmt.Sprintf("  %s%s\n", marker, name))
				}
				m.chat.AddSystemMessage(sb.String())
			} else {
				if err := m.providerReg.Switch(args[0]); err != nil {
					m.chat.AddSystemMessage(fmt.Sprintf("Error: %v", err))
				} else {
					m.chat.AddSystemMessage(fmt.Sprintf("Switched to: %s", args[0]))
				}
			}

		case "model", "m":
			if len(args) == 0 {
				if cur, err := m.providerReg.Current(); err == nil {
					var sb strings.Builder
					sb.WriteString("Available models:\n")
					for _, mi := range cur.Models() {
						sb.WriteString(fmt.Sprintf("  %s - %s (ctx: %d)\n", mi.ID, mi.Name, mi.ContextWindow))
					}
					m.chat.AddSystemMessage(sb.String())
				}
			} else {
				m.chat.AddSystemMessage("Model: " + args[0] + " (applies to next request)")
			}

		case "cost", "c":
			m.chat.AddSystemMessage(fmt.Sprintf(
				"Session Cost\n  Tokens: %d\n  Cost:   $%.4f\n  Time:   %s",
				m.totalTokens, m.totalCost, time.Since(m.startTime).Round(time.Second),
			))

		case "status", "s":
			if m.gitRepo != nil {
				status, err := m.gitRepo.Status()
				if err != nil {
					m.chat.AddSystemMessage(fmt.Sprintf("Git: %v", err))
				} else {
					branch, _ := m.gitRepo.Branch()
					m.chat.AddSystemMessage(fmt.Sprintf("Branch: %s\n%s", branch, status))
				}
			} else {
				m.chat.AddSystemMessage("Not a git repository")
			}

		case "commit":
			if m.gitRepo == nil {
				m.chat.AddSystemMessage("No git repo")
				return nil
			}
			if err := m.gitRepo.AddAll(); err != nil {
				m.chat.AddSystemMessage(fmt.Sprintf("Add: %v", err))
				return nil
			}
			msg := git.GenerateCommitMessage("feat", "", "AI-assisted changes via CodeForge TUI")
			hash, err := m.gitRepo.Commit(msg)
			if err != nil {
				m.chat.AddSystemMessage(fmt.Sprintf("Commit: %v", err))
				return nil
			}
			m.chat.AddSystemMessage(fmt.Sprintf("Committed: %s\nMessage:   %s", hash, msg))

		// ── Agent loop (/act) ─────────────────────────────
		case "act":
			task := strings.Join(args, " ")
			if task == "" {
				m.chat.AddSystemMessage(
					"Usage: /act <task>\n" +
						"Example: /act read main.go and tell me what it does\n" +
						"Example: /act add error handling to handler.go",
				)
				return nil
			}
			return m.chat.SubmitAgent(task)

		// ── Tool shortcuts ────────────────────────────────
		case "read":
			if len(args) == 0 {
				m.chat.AddSystemMessage("Usage: /read <file>")
				return nil
			}
			task := "Read the file " + strings.Join(args, " ") + " and show its contents"
			return m.chat.SubmitAgent(task)

		case "ls":
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			task := "List the directory: " + dir
			return m.chat.SubmitAgent(task)

		case "grep":
			if len(args) == 0 {
				m.chat.AddSystemMessage("Usage: /grep <pattern> [dir]")
				return nil
			}
			task := "Search for pattern " + strings.Join(args, " ") + " in the project"
			return m.chat.SubmitAgent(task)

		case "run":
			if len(args) == 0 {
				m.chat.AddSystemMessage("Usage: /run <command>")
				return nil
			}
			task := "Run this command and show the output: " + strings.Join(args, " ")
			return m.chat.SubmitAgent(task)

		case "quit", "q":
			m.quitting = true
			return tea.Quit

		case "clear":
			m.chat.Clear()

		default:
			m.chat.AddSystemMessage(fmt.Sprintf("Unknown command: /%s  (type /help)", cmd))
		}
		return nil
	}

	m.chat.SetInput(input)
	return m.chat.Submit()
}

// ────────────────────────────────────────────────────────────
// Tea message types
// ────────────────────────────────────────────────────────────

type errMsg struct{ err error }

type StreamOpenedMsg struct {
	Ch         <-chan provider.StreamToken
	FirstToken provider.StreamToken
}

type StreamTickMsg struct {
	Text         string
	Done         bool
	InputTokens  int
	OutputTokens int
	Error        error
}

type AgentOpenedMsg struct {
	Ch    <-chan agent.Event
	First agent.Event
}

type AgentEventMsg struct {
	Ev agent.Event
}

type DiffUpdateMsg struct{ Content string }

type ContextUpdateMsg struct{ Files []string }

// ────────────────────────────────────────────────────────────
// Pump functions (Bubbletea channel → Cmd)
// ────────────────────────────────────────────────────────────

func pumpStream(ch <-chan provider.StreamToken) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		tok, ok := <-ch
		if !ok {
			return StreamTickMsg{Done: true}
		}
		return StreamTickMsg{
			Text:         tok.Text,
			Done:         tok.Done,
			InputTokens:  tok.InputTokens,
			OutputTokens: tok.OutputTokens,
			Error:        tok.Error,
		}
	}
}

func pumpAgent(ch <-chan agent.Event) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return AgentEventMsg{Ev: agent.Event{Kind: agent.EventDone}}
		}
		return AgentEventMsg{Ev: ev}
	}
}

// ────────────────────────────────────────────────────────────
// Pane styling
// ────────────────────────────────────────────────────────────

func paneStyle(active bool) lipgloss.Style {
	color := lipgloss.Color("#334155")
	if active {
		color = lipgloss.Color("#06B6D4")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(color).
		Padding(0, 1)
}

// ────────────────────────────────────────────────────────────
// Helpers
// ────────────────────────────────────────────────────────────

func modeString(m Mode) string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeCommand:
		return "COMMAND"
	}
	return "?"
}

func calculateCost(in, out int, prov string) float64 {
	switch prov {
	case "claude":
		return float64(in)*3.0/1_000_000 + float64(out)*15.0/1_000_000
	default:
		return 0 // Gemini free tier
	}
}

func helpText() string {
	return `CodeForge TUI v0.1.0-alpha  ·  NanoMind 2026  ·  Apache 2.0

MODES
  i          Enter INSERT mode (type messages for streaming chat)
  Esc        Back to NORMAL mode
  :          Open command palette
  /          Open slash command palette

NAVIGATION
  1 / 2 / 3  Focus Chat / Diff / Context pane
  Tab        Cycle panes
  j / k      Scroll down / up
  g / G      Top / bottom

CHAT (streaming, no tools)
  i → type → Enter    Send a chat message
  Ctrl+C              Interrupt current operation

AGENT MODE (tool-calling: reads/writes files, runs commands)
  /act <task>         Start agent loop for a coding task
  /read <file>        Read a file via agent
  /ls [dir]           List directory via agent
  /grep <pat> [dir]   Search code via agent
  /run <cmd>          Run a shell command via agent

GIT
  /status             Show git status
  /commit             Auto-commit staged changes

OTHER
  /provider [name]    List or switch AI provider
  /model [name]       List or switch model
  /cost               Show token & cost summary
  /clear              Clear chat history
  /help               Show this help
  /quit               Exit`
}

func aboutText() string {
	return `CodeForge TUI v0.1.0-alpha
Created by NanoMind — 2026 — Apache 2.0

Stack: Go 1.25 · Bubble Tea · Gemini/Claude/OpenAI
Phase: Fase 1 — Multi-Provider + Agent Loop

"Building the future of terminal AI coding, one keystroke at a time."
                        — NanoMind, 2026`
}

func versionText() string {
	return `CodeForge TUI v0.1.0-alpha
Author:  NanoMind
Year:    2026
License: Apache 2.0
Phase:   Fase 1 (Multi-Provider + Agent Loop)`
}
