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

	// channels
	streamCh <-chan provider.StreamToken
	agentCh  <-chan agent.Event

	quitting    bool
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
	return tea.Batch(
		m.chat.Init(),
		m.context.Init(),
		spinnerTick(), // start spinner
	)
}

// ────────────────────────────────────────────────────────────
// Update
// ────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	// ── Spinner ──────────────────────────────────────────
	case SpinnerTickMsg:
		nc, _ := m.chat.Update(msg)
		m.chat = nc.(ChatModel)
		cmds = append(cmds, spinnerTick())

	// ── Window resize ────────────────────────────────────
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcSizes()

	// ── Key input ────────────────────────────────────────
	case tea.KeyMsg:
		// Global keys always win
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "ctrl+l":
			return m, tea.ClearScreen
		}

		// ── COMMAND mode ─────────────────────────────
		if m.mode == ModeCommand {
			newCmd, c := m.command.Update(msg)
			m.command = newCmd.(CommandModel)
			if c != nil {
				cmds = append(cmds, c)
			}
			if m.command.Done {
				action := m.command.FinalValue
				m.command = NewCommandModel()
				m.mode = ModeNormal
				if c2 := m.executeSlashCommand(action); c2 != nil {
					cmds = append(cmds, c2)
				}
			}
			return m, tea.Batch(cmds...)
		}

		// ── INSERT mode ───────────────────────────────
		if m.mode == ModeInsert {
			switch msg.String() {
			case "esc":
				m.mode = ModeNormal
				return m, nil
			case "enter":
				if m.chat.streaming {
					return m, nil
				}
				inp := strings.TrimSpace(m.chat.input)
				if inp == "" {
					return m, nil
				}
				// BUG FIX #2: route slash commands from INSERT mode
				if strings.HasPrefix(inp, "/") {
					m.chat.input = ""
					m.mode = ModeNormal
					if c := m.executeSlashCommand(inp); c != nil {
						cmds = append(cmds, c)
					}
					return m, tea.Batch(cmds...)
				}
				// Regular chat submit
				if c := m.chat.Submit(); c != nil {
					cmds = append(cmds, c)
				}
				return m, tea.Batch(cmds...)
			case "backspace":
				m.chat.Backspace()
				return m, nil
			case "ctrl+w": // delete last word
				inp := m.chat.input
				inp = strings.TrimRight(inp, " ")
				if i := strings.LastIndex(inp, " "); i >= 0 {
					inp = inp[:i+1]
				} else {
					inp = ""
				}
				m.chat.input = inp
				return m, nil
			case "ctrl+u": // clear line
				m.chat.input = ""
				return m, nil
			case "tab":
				// autocomplete: fill in first matching slash command
				if strings.HasPrefix(m.chat.input, "/") {
					completed := autocomplete(m.chat.input)
					if completed != "" {
						m.chat.input = completed
					}
				}
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				m.chat.TypeText(string(msg.Runes))
				return m, nil
			}
			if len(msg.String()) == 1 {
				m.chat.TypeText(msg.String())
				return m, nil
			}
			return m, nil
		}

		// ── NORMAL mode ───────────────────────────────
		switch msg.String() {
		case "i":
			m.mode = ModeInsert
			return m, nil
		case "I": // insert with /act prefix
			m.mode = ModeInsert
			m.chat.input = "/act "
			return m, nil
		case ":":
			m.mode = ModeCommand
			m.command.Activate()
			return m, nil
		case "/":
			m.mode = ModeInsert
			m.chat.input = "/"
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
		case "esc":
			return m, nil
		case "j", "down", "k", "up", "g", "G":
			nc, c := m.chat.Update(msg)
			m.chat = nc.(ChatModel)
			if c != nil {
				cmds = append(cmds, c)
			}
		}

	// ── Stream (plain chat) ──────────────────────────────
	case StreamOpenedMsg:
		m.streamCh = msg.Ch
		nc, c := m.chat.Update(StreamTickMsg{
			Text:         msg.FirstToken.Text,
			Done:         msg.FirstToken.Done,
			InputTokens:  msg.FirstToken.InputTokens,
			OutputTokens: msg.FirstToken.OutputTokens,
			Error:        msg.FirstToken.Error,
		})
		m.chat = nc.(ChatModel)
		if c != nil {
			cmds = append(cmds, c)
		}
		m.accTokens(msg.FirstToken.InputTokens, msg.FirstToken.OutputTokens)
		if !msg.FirstToken.Done && m.streamCh != nil {
			cmds = append(cmds, pumpStream(m.streamCh))
		} else {
			m.streamCh = nil
		}

	case StreamTickMsg:
		nc, c := m.chat.Update(msg)
		m.chat = nc.(ChatModel)
		if c != nil {
			cmds = append(cmds, c)
		}
		m.accTokens(msg.InputTokens, msg.OutputTokens)
		if !msg.Done && m.streamCh != nil {
			cmds = append(cmds, pumpStream(m.streamCh))
		} else {
			m.streamCh = nil
		}

	// ── Agent loop ───────────────────────────────────────
	case AgentOpenedMsg:
		m.agentCh = msg.Ch
		cmds = append(cmds, m.handleAgentEvent(msg.First)...)
		if msg.First.Kind != agent.EventDone && msg.First.Kind != agent.EventError && m.agentCh != nil {
			cmds = append(cmds, pumpAgent(m.agentCh))
		} else {
			m.agentCh = nil
		}

	case AgentEventMsg:
		cmds = append(cmds, m.handleAgentEvent(msg.Ev)...)
		if msg.Ev.Kind != agent.EventDone && msg.Ev.Kind != agent.EventError && m.agentCh != nil {
			cmds = append(cmds, pumpAgent(m.agentCh))
		} else {
			m.agentCh = nil
		}

	// ── Errors ───────────────────────────────────────────
	case errMsg:
		m.chat.AddSystemMessage("⚠ Error: " + msg.err.Error())
		m.chat.streaming = false
	}

	// Sync status bar setiap update
	m.syncStatus()

	return m, tea.Batch(cmds...)
}

func (m *Model) handleAgentEvent(ev agent.Event) []tea.Cmd {
	var cmds []tea.Cmd

	nc, c := m.chat.Update(AgentEventMsg{Ev: ev})
	m.chat = nc.(ChatModel)
	if c != nil {
		cmds = append(cmds, c)
	}

	if ev.Kind == agent.EventDone {
		m.accTokens(ev.InputTokens, ev.OutputTokens)
	}

	// Update Diff pane kalau write_file menghasilkan diff
	if ev.Kind == agent.EventToolResult && ev.ToolDiff != "" {
		nd, dc := m.diff.Update(DiffUpdateMsg{Content: ev.ToolDiff})
		m.diff = nd.(DiffModel)
		if dc != nil {
			cmds = append(cmds, dc)
		}
		// Auto-switch ke Diff pane sebentar supaya user lihat perubahan
		m.activePane = PaneDiff
		// Context pane: track file yang diubah
		if ev.ToolName == "write_file" {
			// extract filename from tool output "Wrote N bytes to path"
			parts := strings.Fields(ev.ToolOutput)
			if len(parts) >= 4 {
				m.context.AddFile(parts[len(parts)-1])
			}
		}
	}

	// Setelah tool_result, switch balik ke chat
	if ev.Kind == agent.EventDone || ev.Kind == agent.EventError {
		m.activePane = PaneChat
	}

	return cmds
}

func (m *Model) accTokens(in, out int) {
	m.totalTokens += in + out
	m.totalCost += calculateCost(in, out, m.providerReg.CurrentName())
}

func (m *Model) recalcSizes() {
	mainH := m.height - 2
	if mainH < 8 {
		mainH = 8
	}
	// 50% chat, 30% diff, 20% context
	chatW := m.width * 50 / 100
	diffW := m.width * 30 / 100
	ctxW := m.width - chatW - diffW - 6 // 6 = borders
	if chatW < 20 {
		chatW = 20
	}
	if diffW < 10 {
		diffW = 10
	}
	if ctxW < 8 {
		ctxW = 8
	}
	m.chat.SetSize(chatW, mainH)
	m.diff.SetSize(diffW, mainH)
	m.context.SetSize(ctxW, mainH)
	m.status.SetSize(m.width)
	m.command.SetSize(m.width, mainH)
}

func (m *Model) syncStatus() {
	m.status.Mode = modeString(m.mode)
	m.status.Provider = m.providerReg.CurrentName()
	m.status.Tokens = m.totalTokens
	m.status.Cost = m.totalCost
	m.status.Streaming = m.chat.streaming
	if m.gitRepo != nil {
		if branch, err := m.gitRepo.Branch(); err == nil {
			m.status.Branch = branch
		}
	}
	m.status.Workdir = m.workdir
	m.chat.mode = m.mode
}

// ────────────────────────────────────────────────────────────
// View
// ────────────────────────────────────────────────────────────

func (m Model) View() string {
	if m.quitting {
		return "Goodbye! — CodeForge TUI by NanoMind\n"
	}
	if m.width == 0 {
		return "Menginisialisasi...\n"
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
	// Strip leading "/" if present
	raw := input
	if strings.HasPrefix(raw, "/") {
		raw = raw[1:]
	}
	parts := strings.Fields(raw)
	if len(parts) == 0 {
		return nil
	}
	cmd := strings.ToLower(parts[0])
	args := parts[1:]
	argStr := strings.Join(args, " ")

	switch cmd {
	case "help", "h", "?":
		m.chat.AddSystemMessage(helpText())

	case "about":
		m.chat.AddSystemMessage(aboutText())

	case "provider", "p":
		if len(args) == 0 {
			var sb strings.Builder
			sb.WriteString("Provider yang tersedia:\n")
			for _, name := range m.providerReg.List() {
				mark := "  "
				if name == m.providerReg.CurrentName() {
					mark = "* "
				}
				sb.WriteString(fmt.Sprintf("  %s%s\n", mark, name))
			}
			sb.WriteString("\nGanti: /provider gemini  atau  /provider claude")
			m.chat.AddSystemMessage(sb.String())
		} else {
			if err := m.providerReg.Switch(args[0]); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("✓ Provider: " + args[0])
			}
		}

	case "model", "m":
		if len(args) == 0 {
			if cur, err := m.providerReg.Current(); err == nil {
				var sb strings.Builder
				sb.WriteString("Model tersedia:\n")
				for _, mi := range cur.Models() {
					sb.WriteString(fmt.Sprintf("  %s\n    %s  (ctx: %d)\n", mi.ID, mi.Name, mi.ContextWindow))
				}
				m.chat.AddSystemMessage(sb.String())
			}
		} else {
			m.chat.AddSystemMessage("Model akan digunakan: " + argStr)
		}

	case "cost", "c":
		dur := time.Since(m.startTime).Round(time.Second)
		m.chat.AddSystemMessage(fmt.Sprintf(
			"Session Summary\n"+
				"  Provider : %s\n"+
				"  Tokens   : %d\n"+
				"  Biaya    : $%.4f\n"+
				"  Durasi   : %s",
			m.providerReg.CurrentName(),
			m.totalTokens, m.totalCost, dur,
		))

	case "status", "s":
		if m.gitRepo != nil {
			status, err := m.gitRepo.Status()
			if err != nil {
				m.chat.AddSystemMessage("Git: " + err.Error())
			} else {
				branch, _ := m.gitRepo.Branch()
				m.chat.AddSystemMessage("Branch: " + branch + "\n\n" + status)
			}
		} else {
			m.chat.AddSystemMessage("Bukan git repository")
		}

	case "commit":
		if m.gitRepo == nil {
			m.chat.AddSystemMessage("Tidak ada git repo")
			return nil
		}
		if err := m.gitRepo.AddAll(); err != nil {
			m.chat.AddSystemMessage("⚠ git add: " + err.Error())
			return nil
		}
		msg := git.GenerateCommitMessage("feat", "", "AI-assisted changes via CodeForge TUI")
		hash, err := m.gitRepo.Commit(msg)
		if err != nil {
			m.chat.AddSystemMessage("⚠ commit: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage(fmt.Sprintf("✓ Committed: %s\n  %s", hash[:8], msg))

	// ── Agent shortcuts ───────────────────────────────────
	case "act", "a":
		if argStr == "" {
			m.chat.AddSystemMessage("Contoh:\n  /act baca main.go dan jelaskan\n  /act tambahkan error handling ke handler.go\n  /act jalankan go test ./...")
			return nil
		}
		return m.chat.SubmitAgent(argStr)

	case "read", "r":
		if argStr == "" {
			m.chat.AddSystemMessage("Contoh: /read main.go")
			return nil
		}
		return m.chat.SubmitAgent("Baca file " + argStr + " dan tampilkan isinya")

	case "ls", "list":
		dir := "."
		if argStr != "" {
			dir = argStr
		}
		return m.chat.SubmitAgent("Tampilkan isi direktori: " + dir)

	case "grep", "find":
		if argStr == "" {
			m.chat.AddSystemMessage("Contoh: /grep func main")
			return nil
		}
		return m.chat.SubmitAgent("Cari pattern ini di project: " + argStr)

	case "run":
		if argStr == "" {
			m.chat.AddSystemMessage("Contoh: /run go build ./...")
			return nil
		}
		return m.chat.SubmitAgent("Jalankan command ini dan tampilkan hasilnya: " + argStr)

	case "explain", "e":
		if argStr == "" {
			m.chat.AddSystemMessage("Contoh: /explain main.go")
			return nil
		}
		return m.chat.SubmitAgent("Baca dan jelaskan secara detail kode di: " + argStr)

	case "fix":
		if argStr == "" {
			m.chat.AddSystemMessage("Contoh: /fix main.go")
			return nil
		}
		return m.chat.SubmitAgent("Baca file " + argStr + ", temukan bug atau error, lalu perbaiki")

	case "clear":
		m.chat.Clear()
		m.diff = NewDiffModel()

	case "quit", "q", "exit":
		m.quitting = true
		return tea.Quit

	default:
		// Kalau tidak dikenali, tanyakan ke AI
		return m.chat.SubmitAgent(input)
	}
	return nil
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
// Pump functions
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

func spinnerTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// ────────────────────────────────────────────────────────────
// Pane styling
// ────────────────────────────────────────────────────────────

func paneStyle(active bool) lipgloss.Style {
	borderColor := lipgloss.Color("#334155")
	if active {
		borderColor = lipgloss.Color("#06B6D4")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
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

// autocomplete returns the shortest matching slash command for a prefix.
var slashCommands = []string{
	"/act", "/read", "/ls", "/grep", "/run", "/explain", "/fix",
	"/status", "/commit", "/provider", "/model", "/cost",
	"/clear", "/help", "/about", "/quit",
}

func autocomplete(input string) string {
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd, input) {
			return cmd + " "
		}
	}
	return ""
}

// ────────────────────────────────────────────────────────────
// Help & about text
// ────────────────────────────────────────────────────────────

func helpText() string {
	return `CodeForge TUI v0.1.0-alpha  ·  NanoMind 2026

MODES
  i          Masuk INSERT mode (ketik pesan atau /command)
  I          Masuk INSERT mode dengan /act siap diketik
  /          Masuk INSERT mode dengan / siap diketik
  Esc        Kembali ke NORMAL
  :          Command palette

NAVIGATION
  1 / 2 / 3  Fokus Chat / Diff / Context pane
  Tab        Pindah pane
  j / k      Scroll bawah / atas
  g / G      Ke atas / ke bawah

CHAT (streaming)
  i → ketik pesan → Enter    Kirim ke AI (streaming)
  Ctrl+W     Hapus kata terakhir
  Ctrl+U     Hapus semua input
  Tab        Autocomplete /command

AGENT MODE (tool-calling: baca/tulis file, jalankan command)
  /act <task>       Start agent — beri task bebas
  /read <file>      Baca dan tampilkan file
  /ls [dir]         List direktori
  /grep <pattern>   Cari di seluruh project
  /run <command>    Jalankan shell command
  /explain <file>   Jelaskan kode secara detail
  /fix <file>       Temukan dan perbaiki bug

GIT
  /status           Git status
  /commit           Auto-commit perubahan

LAINNYA
  /provider [name]  Ganti AI provider (gemini/claude)
  /model [name]     Ganti model
  /cost             Tampilkan token & biaya session
  /clear            Bersihkan chat
  /help             Tampilkan bantuan ini
  q                 Keluar`
}

func aboutText() string {
	return `CodeForge TUI v0.1.0-alpha
Dibuat oleh NanoMind — 2026 — Apache 2.0

Stack: Go 1.25 · Bubble Tea · Gemini/Claude
Fase: Fase 1 — Multi-Provider + Agent Loop

"Terminal AI coding companion — open, modular, vendor-neutral"
                        — NanoMind, 2026`
}
