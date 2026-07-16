package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/checkpoint"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/git"
	gh "github.com/codeforge/tui/internal/github"
	"github.com/codeforge/tui/internal/index"
	"github.com/codeforge/tui/internal/keymap"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/ui/components"
	"github.com/codeforge/tui/internal/ui/filepicker"
	"github.com/codeforge/tui/internal/ui/palette"
	"github.com/codeforge/tui/internal/ui/review"
)

// Compact breakpoint: single-pane tab mode below this width.
const compactBreakpoint = 100

type Model struct {
	cfg         *config.Config
	providerReg *provider.Registry
	toolReg     *tool.Registry
	gitRepo     *git.Repo
	workdir     string
	keys        keymap.Map

	width      int
	height     int
	activePane Pane
	mode       Mode
	agentMode  tool.WriteMode // Plan default
	compact    bool           // single-pane mode

	chat    ChatModel
	diff    DiffModel
	context ContextModel
	status  StatusBarModel
	command CommandModel
	palette palette.Model
	picker  filepicker.Model
	review  review.Model
	toast   components.Toast

	streamCh <-chan provider.StreamToken
	agentCh  <-chan agent.Event

	session   *session.Session
	ghClient  *gh.Client
	quitting  bool
	startTime time.Time
	totalCost float64
	totalTokens int

	// motion
	borderPhase float64
	lastTokenAt time.Time
	tokenWindow int
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
	ModePalette
	ModeFilePick
	ModeReview
)

func New(cfg *config.Config, provReg *provider.Registry, toolReg *tool.Registry, repo *git.Repo, workdir string) Model {
	sess := session.New(provReg.CurrentName(), "", workdir)
	if cur, err := provReg.Current(); err == nil {
		sess.Model = cur.Model()
	}
	ghc := gh.New(workdir)
	chat := NewChatModel(provReg, toolReg, repo, workdir)
	// Project rules
	rb := rules.Get()
	if rb != nil && rb.Text != "" {
		chat.SetRules(rb.Text)
	}
	m := Model{
		cfg:         cfg,
		providerReg: provReg,
		toolReg:     toolReg,
		gitRepo:     repo,
		workdir:     workdir,
		keys:        keymap.Default(),
		activePane:  PaneChat,
		mode:        ModeNormal,
		agentMode:   tool.ModePlan,
		startTime:   time.Now(),
		chat:        chat,
		diff:        NewDiffModel(),
		context:     NewContextModel(workdir),
		status:      NewStatusBarModel(),
		command:     NewCommandModel(),
		palette:     palette.New(),
		picker:      filepicker.New(workdir),
		review:      review.New(),
		session:     sess,
		ghClient:    ghc,
	}
	// Ensure staged writer is in Plan mode
	if sw := toolReg.GetStagedWriter(); sw != nil {
		sw.SetMode(tool.ModePlan)
	}
	if rb != nil && len(rb.Paths) > 0 {
		m.chat.AddSystemMessage(rb.Summary())
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.chat.Init(),
		m.context.Init(),
		spinnerTick(),
		// Kick context pane live on start
		func() tea.Msg {
			return ContextUpdateMsg{Refresh: true}
		},
		// Resolve GitHub identity asynchronously
		func() tea.Msg {
			if m.ghClient == nil {
				return GitHubStatusMsg{}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			user, err := m.ghClient.WhoAmI(ctx)
			if err != nil {
				return GitHubStatusMsg{Err: err.Error()}
			}
			slug, _ := m.ghClient.RepoSlug(ctx)
			return GitHubStatusMsg{User: user, Repo: slug, OK: true}
		},
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case SpinnerTickMsg:
		if theme.MotionEnabled() {
			m.borderPhase += 0.02
			if m.borderPhase > 1 {
				m.borderPhase -= 1
			}
		}
		// token sparkline sample
		if time.Since(m.lastTokenAt) < 2*time.Second && m.tokenWindow > 0 {
			m.status.PushSpark(float64(m.tokenWindow) / 500.0)
			m.tokenWindow = 0
		} else {
			m.status.PushSpark(0)
		}
		nc, _ := m.chat.Update(msg)
		m.chat = nc.(ChatModel)
		cmds = append(cmds, spinnerTick())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.compact = msg.Width < compactBreakpoint
		m.recalcSizes()
		m.palette.SetSize(msg.Width, msg.Height)
		m.picker.Width = min(50, msg.Width-4)
		m.review.Width = msg.Width
		m.review.Height = msg.Height - 2

	case tea.KeyMsg:
		// Global
		switch msg.String() {
		case "ctrl+c":
			m.saveSession()
			m.quitting = true
			return m, tea.Quit
		case "ctrl+l":
			return m, tea.ClearScreen
		}

		// ── Review mode ─────────────────────────────
		if m.mode == ModeReview {
			return m.updateReview(msg)
		}

		// ── Palette ─────────────────────────────────
		if m.mode == ModePalette {
			return m.updatePalette(msg)
		}

		// ── File picker ─────────────────────────────
		if m.mode == ModeFilePick {
			return m.updatePicker(msg)
		}

		// ── Command mode ────────────────────────────
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
			m.syncStatus()
			return m, tea.Batch(cmds...)
		}

		// ── INSERT mode ─────────────────────────────
		if m.mode == ModeInsert {
			switch msg.String() {
			case "esc":
				m.mode = ModeNormal
				m.chat.BlurInput()
				m.chat.mode = ModeNormal
				return m, nil
			case "enter":
				if m.chat.streaming {
					return m, nil
				}
				inp := strings.TrimSpace(m.chat.InputValue())
				if inp == "" {
					return m, nil
				}
				if strings.HasPrefix(inp, "/") {
					m.chat.ClearInput()
					m.mode = ModeNormal
					m.chat.BlurInput()
					if c := m.executeSlashCommand(inp); c != nil {
						cmds = append(cmds, c)
					}
					return m, tea.Batch(cmds...)
				}
				if m.budgetBlocks() {
					m.chat.AddSystemMessage("⛔ Budget exceeded — agent/chat blocked (see /budget)")
					return m, nil
				}
				if c := m.chat.Submit(); c != nil {
					cmds = append(cmds, c)
					cmds = append(cmds, m.persistSessionCmd())
				}
				return m, tea.Batch(cmds...)
			case "@":
				// open file picker; also type @ into input after selection
				m.picker.Open()
				m.mode = ModeFilePick
				return m, nil
			case "ctrl+k":
				m.openPalette()
				return m, nil
			}
			// forward to chat textarea
			nc, c := m.chat.Update(msg)
			m.chat = nc.(ChatModel)
			if c != nil {
				cmds = append(cmds, c)
			}
			m.syncStatus()
			return m, tea.Batch(cmds...)
		}

		// ── NORMAL mode ─────────────────────────────
		switch msg.String() {
		case "i":
			m.mode = ModeInsert
			m.chat.mode = ModeInsert
			m.chat.FocusInput()
			return m, nil
		case "I":
			m.mode = ModeInsert
			m.chat.mode = ModeInsert
			m.chat.SetInput("/act ")
			m.chat.FocusInput()
			return m, nil
		case ":":
			m.mode = ModeCommand
			m.command.Activate()
			return m, nil
		case "/":
			m.mode = ModeInsert
			m.chat.mode = ModeInsert
			m.chat.SetInput("/")
			m.chat.FocusInput()
			return m, nil
		case "ctrl+k":
			m.openPalette()
			return m, nil
		case "P": // Shift+P — toggle Plan/Act
			m.toggleAgentMode()
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
			m.saveSession()
			m.quitting = true
			return m, tea.Quit
		case "?":
			m.chat.AddSystemMessage(helpText())
			return m, nil
		case "j", "down", "k", "up", "g", "G", "pgup", "pgdown", "ctrl+d", "ctrl+u":
			if m.activePane == PaneDiff {
				nd, c := m.diff.Update(msg)
				m.diff = nd.(DiffModel)
				if c != nil {
					cmds = append(cmds, c)
				}
			} else {
				nc, c := m.chat.Update(msg)
				m.chat = nc.(ChatModel)
				if c != nil {
					cmds = append(cmds, c)
				}
			}
		case "n", "p":
			if m.activePane == PaneDiff {
				nd, c := m.diff.Update(msg)
				m.diff = nd.(DiffModel)
				if c != nil {
					cmds = append(cmds, c)
				}
			}
		}

	case StreamOpenedMsg:
		m.streamCh = msg.Ch
		nc, c := m.chat.Update(StreamTickMsg{
			Text: msg.FirstToken.Text, Done: msg.FirstToken.Done,
			InputTokens: msg.FirstToken.InputTokens, OutputTokens: msg.FirstToken.OutputTokens,
			Error: msg.FirstToken.Error,
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
			cmds = append(cmds, m.persistSessionCmd())
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
			cmds = append(cmds, m.persistSessionCmd())
		}

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

	case ContextUpdateMsg:
		nc, c := m.context.Update(msg)
		m.context = nc.(ContextModel)
		if c != nil {
			cmds = append(cmds, c)
		}

	case ToastMsg:
		m.toast = components.NewToast(msg.Text, msg.Kind, 3*time.Second)

	case GitHubStatusMsg:
		if msg.OK {
			m.status.GitHubUser = msg.User
			m.status.GitHubRepo = msg.Repo
			m.status.GitHubOK = true
		} else {
			m.status.GitHubOK = false
			if msg.Err != "" {
				m.status.GitHubUser = ""
			}
		}

	case BabysitDoneMsg:
		body := gh.FormatCheckStatus(msg.Status)
		if msg.Err != nil {
			m.chat.AddSystemMessage("PR babysit finished with issues:\n" + body + "\n⚠ " + msg.Err.Error())
			m.toast = components.NewToast("CI not green", "error", 4*time.Second)
			if msg.Fix {
				// Kick agent to fix failing CI
				task := fmt.Sprintf(
					"CI checks failed on PR #%d. Inspect failures, fix the code with search_replace/apply_patch, run tests, push, then re-check with github babysit_once. Summary:\n%s",
					msg.PR, body,
				)
				if c := m.chat.SubmitAgent(task); c != nil {
					cmds = append(cmds, c)
				}
			}
		} else {
			m.chat.AddSystemMessage("✅ PR babysit complete — checks green\n" + body)
			m.toast = components.NewToast("CI green", "success", 3*time.Second)
		}

	case errMsg:
		m.chat.AddSystemMessage("⚠ Error: " + msg.err.Error())
		m.chat.streaming = false
		m.toast = components.NewToast(msg.err.Error(), "error", 4*time.Second)
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
		m.chat.FocusInput()
		return m, nil
	case "enter":
		m.picker.Confirm()
		m.mode = ModeInsert
		m.chat.FocusInput()
		if m.picker.Selected != "" {
			rel := m.picker.Selected
			// insert @file into textarea
			cur := m.chat.InputValue()
			m.chat.SetInput(cur + "@" + rel + " ")
			if body, err := filepicker.ReadFileContent(m.workdir, rel, 32_000); err == nil {
				m.chat.AttachFile(rel, body)
				m.context.MarkTouched(rel)
			}
		}
		return m, nil
	case "up", "k":
		m.picker.Move(-1)
	case "down", "j":
		m.picker.Move(1)
	case "backspace":
		m.picker.Backspace()
	default:
		if msg.Type == tea.KeyRunes {
			m.picker.Type(string(msg.Runes))
		}
	}
	return m, nil
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

func (m *Model) toggleAgentMode() {
	sw := m.toolReg.GetStagedWriter()
	if m.agentMode == tool.ModePlan {
		m.agentMode = tool.ModeAct
		if sw != nil {
			sw.SetMode(tool.ModeAct)
		}
		m.toast = components.NewToast("Mode: ACT — writes apply immediately", "warning", 3*time.Second)
		m.chat.AddSystemMessage("⚡ Mode ACT: write_file menulis langsung ke disk.")
	} else {
		m.agentMode = tool.ModePlan
		if sw != nil {
			sw.SetMode(tool.ModePlan)
		}
		m.toast = components.NewToast("Mode: PLAN — writes require review", "info", 3*time.Second)
		m.chat.AddSystemMessage("🛡 Mode PLAN: write_file di-stage untuk review.")
	}
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
		// Plan mode: open review if pending
		if sw := m.toolReg.GetStagedWriter(); sw != nil && sw.HasPending() {
			patches := sw.Pending()
			m.review.Open(patches)
			m.mode = ModeReview
			// show combined diff
			var combined string
			for _, p := range patches {
				combined += p.Diff + "\n"
			}
			nd, _ := m.diff.Update(DiffUpdateMsg{Content: combined, Pending: true})
			m.diff = nd.(DiffModel)
			m.activePane = PaneDiff
		} else {
			m.activePane = PaneChat
		}
		cmds = append(cmds, m.persistSessionCmd())
	}
	if ev.Kind == agent.EventToolResult && ev.ToolDiff != "" {
		nd, dc := m.diff.Update(DiffUpdateMsg{Content: ev.ToolDiff, Pending: m.agentMode == tool.ModePlan})
		m.diff = nd.(DiffModel)
		if dc != nil {
			cmds = append(cmds, dc)
		}
		m.activePane = PaneDiff
		if ev.ToolName == "write_file" {
			parts := strings.Fields(ev.ToolOutput)
			if len(parts) >= 1 {
				// last token often path
				path := parts[len(parts)-1]
				m.context.MarkTouched(path)
				// Act mode: checkpoint immediately
				if m.agentMode == tool.ModeAct {
					abs := path
					if !filepath.IsAbs(abs) {
						abs = filepath.Join(m.workdir, path)
					}
					old := ""
					// old content already overwritten — checkpoint best-effort empty for new
					_, _ = checkpoint.Save(m.session.ID, abs, path, old)
				}
			}
		}
	}
	if ev.Kind == agent.EventToolCall && (ev.ToolName == "read_file" || ev.ToolName == "write_file") {
		// extract path from JSON-ish input
		if p := extractPath(ev.ToolInput); p != "" {
			m.context.MarkTouched(p)
		}
	}
	if ev.Kind == agent.EventError {
		m.activePane = PaneChat
	}
	return cmds
}

func extractPath(toolInput string) string {
	// crude: "path":"foo"
	const key = `"path"`
	i := strings.Index(toolInput, key)
	if i < 0 {
		return ""
	}
	rest := toolInput[i+len(key):]
	// find quoted value
	q1 := strings.Index(rest, `"`)
	if q1 < 0 {
		return ""
	}
	rest = rest[q1+1:]
	q2 := strings.Index(rest, `"`)
	if q2 < 0 {
		return ""
	}
	return rest[:q2]
}

func (m *Model) accTokens(in, out int) {
	m.totalTokens += in + out
	m.tokenWindow += in + out
	m.lastTokenAt = time.Now()
	cost := 0.0
	if cur, err := m.providerReg.Current(); err == nil {
		cost = provider.CostForModel(cur, cur.Model(), in, out)
	}
	m.totalCost += cost
	m.checkBudget()
}

func (m *Model) checkBudget() {
	if m.cfg == nil {
		return
	}
	max := m.cfg.Budget.MaxCostUSD
	warn := m.cfg.Budget.WarnAtUSD
	if max <= 0 {
		return
	}
	if warn <= 0 {
		warn = max * 0.5
	}
	m.status.BudgetMax = max
	m.status.BudgetWarn = m.totalCost >= warn
	m.status.BudgetStop = m.totalCost >= max
	if m.status.BudgetStop {
		m.toast = components.NewToast(fmt.Sprintf("Budget exceeded $%.2f — agent blocked", max), "error", 5*time.Second)
	} else if m.status.BudgetWarn && m.totalCost-costEpsilon() < warn {
		// only toast once-ish when crossing
		m.toast = components.NewToast(fmt.Sprintf("Budget warning $%.4f / $%.2f", m.totalCost, max), "warning", 3*time.Second)
	}
}

func costEpsilon() float64 { return 0.00001 }

// budgetBlocks returns true if further paid AI calls should be refused.
func (m *Model) budgetBlocks() bool {
	return m.cfg != nil && m.cfg.Budget.MaxCostUSD > 0 && m.totalCost >= m.cfg.Budget.MaxCostUSD
}

func (m *Model) recalcSizes() {
	mainH := m.height - 2
	if m.toast.Alive() {
		mainH--
	}
	if mainH < 8 {
		mainH = 8
	}
	if m.compact {
		// full width for active pane
		w := m.width - 4
		m.chat.SetSize(w, mainH)
		m.diff.SetSize(w, mainH)
		m.context.SetSize(w, mainH)
	} else {
		chatW := m.width * 50 / 100
		diffW := m.width * 30 / 100
		ctxW := m.width - chatW - diffW - 6
		if chatW < 20 {
			chatW = 20
		}
		if diffW < 12 {
			diffW = 12
		}
		if ctxW < 10 {
			ctxW = 10
		}
		m.chat.SetSize(chatW, mainH)
		m.diff.SetSize(diffW, mainH)
		m.context.SetSize(ctxW, mainH)
	}
	m.status.SetSize(m.width)
	m.command.SetSize(m.width, mainH)
}

func (m *Model) syncStatus() {
	m.status.Mode = modeString(m.mode)
	m.status.Provider = m.providerReg.CurrentName()
	if cur, err := m.providerReg.Current(); err == nil {
		m.status.ModelName = cur.Model()
	}
	m.status.Tokens = m.totalTokens
	m.status.Cost = m.totalCost
	m.status.Streaming = m.chat.streaming
	if m.cfg != nil {
		m.status.BudgetMax = m.cfg.Budget.MaxCostUSD
		if m.cfg.Budget.MaxCostUSD > 0 {
			warn := m.cfg.Budget.WarnAtUSD
			if warn <= 0 {
				warn = m.cfg.Budget.MaxCostUSD * 0.5
			}
			m.status.BudgetWarn = m.totalCost >= warn
			m.status.BudgetStop = m.totalCost >= m.cfg.Budget.MaxCostUSD
		}
	}
	if m.agentMode == tool.ModePlan {
		m.status.AgentMode = "PLAN"
	} else {
		m.status.AgentMode = "ACT"
	}
	if m.gitRepo != nil {
		if branch, err := m.gitRepo.Branch(); err == nil {
			m.status.Branch = branch
		}
	}
	m.status.Workdir = m.workdir
	m.chat.mode = m.mode
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

func (m Model) View() string {
	if m.quitting {
		return "Goodbye! — CodeForge TUI by NanoMind\n"
	}
	if m.width == 0 {
		return "Menginisialisasi...\n"
	}

	topBar := m.status.ViewTop()
	bottomBar := m.status.ViewBottom()

	var mainRow string
	if m.compact {
		// single pane
		var body string
		switch m.activePane {
		case PaneDiff:
			body = theme.PaneBorder(true, m.width-2, m.height-4).Render(m.diff.View())
		case PaneContext:
			body = theme.PaneBorder(true, m.width-2, m.height-4).Render(m.context.View())
		default:
			body = theme.PaneBorder(true, m.width-2, m.height-4).Render(m.chat.View())
		}
		tabHint := lipgloss.NewStyle().Foreground(theme.Current().TextMuted).Render(
			fmt.Sprintf("  [1]Chat [2]Diff [3]Files  ·  focus: %s", paneName(m.activePane)))
		mainRow = lipgloss.JoinVertical(lipgloss.Left, tabHint, body)
	} else {
		chatStyle := theme.PaneBorder(m.activePane == PaneChat, 0, 0)
		diffStyle := theme.PaneBorder(m.activePane == PaneDiff, 0, 0)
		ctxStyle := theme.PaneBorder(m.activePane == PaneContext, 0, 0)
		// re-apply size via width on join
		mainRow = lipgloss.JoinHorizontal(lipgloss.Top,
			chatStyle.Render(m.chat.View()),
			diffStyle.Render(m.diff.View()),
			ctxStyle.Render(m.context.View()),
		)
	}

	parts := []string{topBar}
	if m.toast.Alive() {
		parts = append(parts, m.toast.View(m.width))
	}
	parts = append(parts, mainRow)

	switch m.mode {
	case ModeCommand:
		parts = append(parts, m.command.View())
	case ModePalette:
		// overlay palette centered-ish
		parts = append(parts, m.palette.View())
	case ModeFilePick:
		parts = append(parts, m.picker.View())
	case ModeReview:
		return lipgloss.JoinVertical(lipgloss.Left, topBar, m.review.View(), bottomBar)
	}
	parts = append(parts, bottomBar)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
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

func (m *Model) executeSlashCommand(input string) tea.Cmd {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
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
			sb.WriteString("Provider tersedia:\n")
			for _, name := range m.providerReg.List() {
				mark := "  "
				if name == m.providerReg.CurrentName() {
					mark = "* "
				}
				sb.WriteString(fmt.Sprintf("  %s%s\n", mark, name))
			}
			sb.WriteString("\nGanti: /provider gemini | claude | openai | ollama")
			m.chat.AddSystemMessage(sb.String())
		} else {
			if err := m.providerReg.Switch(args[0]); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("✓ Provider: " + args[0])
				m.toast = components.NewToast("Provider → "+args[0], "success", 2*time.Second)
			}
		}

	case "model", "m":
		if len(args) == 0 {
			if cur, err := m.providerReg.Current(); err == nil {
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("Model aktif: %s\n\nModel tersedia:\n", cur.Model()))
				for _, mi := range cur.Models() {
					mark := "  "
					if mi.ID == cur.Model() {
						mark = "* "
					}
					sb.WriteString(fmt.Sprintf("%s%s\n    %s  ctx:%d  $%.2f/$%.2f per 1M\n",
						mark, mi.ID, mi.Name, mi.ContextWindow, mi.InputCost, mi.OutputCost))
				}
				sb.WriteString("\nGanti: /model <id>")
				m.chat.AddSystemMessage(sb.String())
			}
		} else {
			// ROOT FIX: actually switch model via SetModel
			if cur, err := m.providerReg.Current(); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else if err := cur.SetModel(argStr); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("✓ Model: " + argStr)
				m.toast = components.NewToast("Model → "+argStr, "success", 2*time.Second)
				if m.session != nil {
					m.session.Model = argStr
				}
			}
		}

	case "mode":
		if len(args) == 0 {
			name := "PLAN"
			if m.agentMode == tool.ModeAct {
				name = "ACT"
			}
			m.chat.AddSystemMessage(fmt.Sprintf("Agent write mode: %s\n  /mode plan  — stage writes for review (default)\n  /mode act   — write immediately\n  Shift+P     — toggle", name))
		} else {
			switch strings.ToLower(args[0]) {
			case "plan":
				if m.agentMode != tool.ModePlan {
					m.toggleAgentMode()
				}
			case "act":
				if m.agentMode != tool.ModeAct {
					m.toggleAgentMode()
				}
			default:
				m.chat.AddSystemMessage("Gunakan: /mode plan | act")
			}
		}

	case "sessions":
		if len(args) == 0 {
			list, err := session.List(15)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			if len(list) == 0 {
				m.chat.AddSystemMessage("Belum ada sesi tersimpan.")
				return nil
			}
			var sb strings.Builder
			sb.WriteString("Sesi tersimpan (resume: /sessions <id>):\n")
			for _, s := range list {
				sb.WriteString(fmt.Sprintf("  %s  %s\n    %s\n", s.ID, s.Slug, s.Preview))
			}
			m.chat.AddSystemMessage(sb.String())
		} else {
			s, err := session.Load(args[0])
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
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
			m.chat.AddSystemMessage(fmt.Sprintf("✓ Resumed session %s", s.ID))
			m.toast = components.NewToast("Session resumed", "success", 2*time.Second)
		}

	case "undo":
		if m.session == nil {
			m.chat.AddSystemMessage("No session for undo")
			return nil
		}
		rel, err := checkpoint.UndoLast(m.session.ID)
		if err != nil {
			m.chat.AddSystemMessage("⚠ undo: " + err.Error())
		} else {
			m.chat.AddSystemMessage("✓ Restored: " + rel)
			m.toast = components.NewToast("Undid "+rel, "success", 3*time.Second)
			m.context.MarkTouched(rel)
		}

	case "cost", "c":
		dur := time.Since(m.startTime).Round(time.Second)
		m.chat.AddSystemMessage(fmt.Sprintf(
			"Session Summary\n  Provider : %s\n  Tokens   : %d\n  Biaya    : $%.4f\n  Durasi   : %s\n  Mode     : %s",
			m.providerReg.CurrentName(), m.totalTokens, m.totalCost, dur, m.status.AgentMode,
		))

	case "status", "s":
		if m.gitRepo != nil {
			status, err := m.gitRepo.Status()
			if err != nil {
				m.chat.AddSystemMessage("Git: " + err.Error())
			} else {
				branch, _ := m.gitRepo.Branch()
				m.chat.AddSystemMessage("Branch: " + branch + "\n\n" + status)
				// refresh context with git glyphs
				gs := parseGitStatus(status)
				nc, _ := m.context.Update(ContextUpdateMsg{Refresh: true, GitStatus: gs})
				m.context = nc.(ContextModel)
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
		cmsg := argStr
		if cmsg == "" {
			cmsg = git.GenerateCommitMessage("feat", "", "AI-assisted changes via CodeForge TUI")
		}
		hash, err := m.gitRepo.Commit(cmsg)
		if err != nil {
			m.chat.AddSystemMessage("⚠ commit: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage(fmt.Sprintf("✓ Committed: %s\n  %s", hash, cmsg))
		m.toast = components.NewToast("Committed "+hash, "success", 3*time.Second)

	// ── GitHub integration (gh + API) ─────────────────────
	case "gh", "github":
		return m.handleGitHubCommand(args)

	case "pr":
		// /pr [list|view|create|merge|checks] ...
		if len(args) == 0 {
			return m.handleGitHubCommand([]string{"pr", "list"})
		}
		return m.handleGitHubCommand(append([]string{"pr"}, args...))

	case "issue":
		if len(args) == 0 {
			return m.handleGitHubCommand([]string{"issue", "list"})
		}
		return m.handleGitHubCommand(append([]string{"issue"}, args...))

	case "push":
		return m.handleGitHubCommand([]string{"push"})

	case "pull":
		return m.handleGitHubCommand([]string{"pull"})

	case "act", "a":
		if argStr == "" {
			m.chat.AddSystemMessage("Examples:\n  /act explain auth flow\n  /act fix failing tests")
			return nil
		}
		if m.budgetBlocks() {
			m.chat.AddSystemMessage("⛔ Budget exceeded — raise budget.max_cost_usd or /clear cost via new session")
			return nil
		}
		return m.chat.SubmitAgent(argStr)

	case "rules":
		rb := rules.Get()
		if rb == nil || rb.Text == "" {
			m.chat.AddSystemMessage("No project rules. Create AGENTS.md or .codeforge/rules.md in the project root.")
			return nil
		}
		m.chat.AddSystemMessage(rb.Summary() + "\n\n" + rb.Text)
		return nil

	case "index":
		idx := index.Global()
		if idx == nil {
			m.chat.AddSystemMessage("Index not ready — building…")
			return func() tea.Msg {
				built, err := index.Build(m.workdir)
				if err != nil {
					return errMsg{err: err}
				}
				index.SetGlobal(built)
				f, s := built.Stats()
				return ToastMsg{Text: fmt.Sprintf("Index ready: %d files, %d symbols", f, s), Kind: "success"}
			}
		}
		f, s := idx.Stats()
		m.chat.AddSystemMessage(fmt.Sprintf("Codebase index: %d files, %d symbols\nUse agent tool codebase_search or /act where is X?", f, s))
		return nil

	case "budget":
		if m.cfg == nil {
			m.chat.AddSystemMessage("No config")
			return nil
		}
		m.chat.AddSystemMessage(fmt.Sprintf(
			"Session cost: $%.4f\nBudget max: $%.2f (0=unlimited)\nWarn at: $%.2f\nTokens: %d\n\nSet in config:\n  budget:\n    max_cost_usd: 1.0\n    warn_at_usd: 0.5",
			m.totalCost, m.cfg.Budget.MaxCostUSD, m.cfg.Budget.WarnAtUSD, m.totalTokens,
		))
		return nil

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
		m.session = session.New(m.providerReg.CurrentName(), "", m.workdir)

	case "quit", "q", "exit":
		m.saveSession()
		m.quitting = true
		return tea.Quit

	default:
		return m.chat.SubmitAgent(input)
	}
	return nil
}

// handleGitHubCommand runs GitHub ops from slash commands.
// Forms:
//
//	/gh auth|repo|push|pull|log|branch <name>
//	/gh pr list|view [n]|create <title> [| body]|merge <n>|checks [n]
//	/gh issue list|view <n>|create <title> [| body]
//	/pr … and /issue … are aliases without the leading "pr"/"issue" word duplicated.
func (m *Model) handleGitHubCommand(args []string) tea.Cmd {
	if m.ghClient == nil {
		m.chat.AddSystemMessage("GitHub client not available")
		return nil
	}
	if len(args) == 0 {
		m.chat.AddSystemMessage(githubHelpText())
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	// note: cancel after sync work; long ops still bounded
	defer cancel()

	head := strings.ToLower(args[0])
	rest := args[1:]

	// Allow /gh pr … and also when called as /pr via prepend
	switch head {
	case "help", "h", "?":
		m.chat.AddSystemMessage(githubHelpText())
		return nil

	case "auth", "status":
		out, err := m.ghClient.AuthStatus(ctx)
		if err != nil {
			m.chat.AddSystemMessage("⚠ GitHub auth: " + err.Error() +
				"\n\nSetup:\n  gh auth login\n  — or —\n  export GITHUB_TOKEN=ghp_...")
			return nil
		}
		user, _ := m.ghClient.WhoAmI(ctx)
		slug, _ := m.ghClient.RepoSlug(ctx)
		m.status.GitHubUser = user
		m.status.GitHubRepo = slug
		m.status.GitHubOK = true
		m.chat.AddSystemMessage("GitHub auth:\n" + out +
			fmt.Sprintf("\n\nUser: %s\nRepo: %s", user, slug))
		return nil

	case "repo":
		out, err := m.ghClient.RepoView(ctx)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Repository:\n" + out)
		return nil

	case "push":
		out, err := m.ghClient.Push(ctx, true)
		if err != nil {
			m.chat.AddSystemMessage("⚠ push: " + err.Error())
			m.toast = components.NewToast("Push failed", "error", 3*time.Second)
			return nil
		}
		m.chat.AddSystemMessage("✓ Pushed\n" + out)
		m.toast = components.NewToast("Pushed to origin", "success", 3*time.Second)
		return nil

	case "pull":
		out, err := m.ghClient.Pull(ctx)
		if err != nil {
			m.chat.AddSystemMessage("⚠ pull: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Pulled\n" + out)
		m.toast = components.NewToast("Pulled", "success", 2*time.Second)
		return nil

	case "log":
		out, err := m.ghClient.LogRecent(ctx, 15)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Recent commits:\n" + out)
		return nil

	case "branch":
		if len(rest) == 0 {
			br, err := m.ghClient.CurrentBranch(ctx)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage("Current branch: " + br)
			return nil
		}
		name := rest[0]
		out, err := m.ghClient.CreateBranch(ctx, name)
		if err != nil {
			m.chat.AddSystemMessage("⚠ branch: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Branch: " + name + "\n" + out)
		m.toast = components.NewToast("Branch "+name, "success", 2*time.Second)
		return nil

	case "pr":
		return m.handlePRSubcommand(ctx, rest)

	case "issue":
		return m.handleIssueSubcommand(ctx, rest)

	case "checks":
		return m.handlePRSubcommand(ctx, append([]string{"checks"}, rest...))

	default:
		// Treat unknown as agent task about github
		return m.chat.SubmitAgent("GitHub task: " + strings.Join(args, " ") +
			" — use the github tool with the appropriate action.")
	}
}

func (m *Model) handlePRSubcommand(ctx context.Context, args []string) tea.Cmd {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch sub {
	case "list", "ls":
		state := "open"
		if len(rest) > 0 {
			state = rest[0]
		}
		prs, err := m.ghClient.ListPRs(ctx, state, 20)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage(gh.FormatPRList(prs))
		return nil
	case "view", "show":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.ViewPR(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Pull request:\n" + out)
		return nil
	case "create", "new":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage:\n  /pr create <title>\n  /pr create <title> | <body markdown>\n  /gh pr create \"title\" (agent can also open PRs via github tool)")
			return nil
		}
		joined := strings.Join(rest, " ")
		title, body := joined, ""
		if i := strings.Index(joined, " | "); i >= 0 {
			title = strings.TrimSpace(joined[:i])
			body = strings.TrimSpace(joined[i+3:])
		}
		out, err := m.ghClient.CreatePR(ctx, title, body, "", "", false)
		if err != nil {
			m.chat.AddSystemMessage("⚠ pr create: " + err.Error())
			m.toast = components.NewToast("PR create failed", "error", 3*time.Second)
			return nil
		}
		m.chat.AddSystemMessage("✓ Pull request created\n" + out)
		m.toast = components.NewToast("PR created", "success", 3*time.Second)
		return nil
	case "merge":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage: /pr merge <number> [squash|merge|rebase]")
			return nil
		}
		n, err := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		if err != nil {
			m.chat.AddSystemMessage("⚠ invalid PR number")
			return nil
		}
		method := "squash"
		if len(rest) > 1 {
			method = rest[1]
		}
		out, err := m.ghClient.MergePR(ctx, n, method)
		if err != nil {
			m.chat.AddSystemMessage("⚠ merge: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Merged PR #" + strconv.Itoa(n) + "\n" + out)
		m.toast = components.NewToast("Merged #"+strconv.Itoa(n), "success", 3*time.Second)
		return nil
	case "checks", "ci":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.Checks(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ checks: " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("CI checks:\n" + out)
		return nil
	case "diff":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.PRDiff(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ pr diff: " + err.Error())
			return nil
		}
		if len(out) > 8000 {
			out = out[:8000] + "\n… (truncated)"
		}
		m.chat.AddSystemMessage("PR diff:\n" + out)
		return nil
	case "comment":
		if len(rest) < 2 {
			m.chat.AddSystemMessage("Usage: /pr comment <number> <body…>")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		body := strings.Join(rest[1:], " ")
		out, err := m.ghClient.CommentOnPR(ctx, n, body)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Comment posted\n" + out)
		m.toast = components.NewToast("PR comment posted", "success", 2*time.Second)
		return nil
	case "review", "reviewers":
		if len(rest) < 2 {
			m.chat.AddSystemMessage("Usage: /pr review <number> user1,user2")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		var revs []string
		for _, r := range strings.Split(strings.Join(rest[1:], " "), ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				revs = append(revs, r)
			}
		}
		out, err := m.ghClient.RequestReviewers(ctx, n, revs)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Reviewers requested\n" + out)
		return nil
	case "commits":
		n := 0
		if len(rest) > 0 {
			n, _ = strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		}
		out, err := m.ghClient.PRCommits(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("PR commits:\n" + out)
		return nil
	case "babysit":
		// /pr babysit [n] [--fix]
		n := 0
		fix := false
		for _, a := range rest {
			if a == "--fix" || a == "fix" {
				fix = true
				continue
			}
			if v, err := strconv.Atoi(strings.TrimPrefix(a, "#")); err == nil {
				n = v
			}
		}
		m.chat.AddSystemMessage(fmt.Sprintf("⏳ Babysitting PR checks (pr=%d)… poll every 20s", n))
		m.toast = components.NewToast("PR babysit started", "info", 2*time.Second)
		ghc := m.ghClient
		return func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			cs, err := ghc.Babysit(ctx, gh.BabysitOptions{
				PRNumber: n,
				Interval: 20 * time.Second,
				Timeout:  15 * time.Minute,
			})
			return BabysitDoneMsg{Status: cs, Err: err, PR: n, Fix: fix}
		}
	default:
		m.chat.AddSystemMessage("Unknown /pr subcommand. " + githubHelpText())
		return nil
	}
}

func (m *Model) handleIssueSubcommand(ctx context.Context, args []string) tea.Cmd {
	if len(args) == 0 {
		args = []string{"list"}
	}
	sub := strings.ToLower(args[0])
	rest := args[1:]
	switch sub {
	case "list", "ls":
		state := "open"
		if len(rest) > 0 {
			state = rest[0]
		}
		issues, err := m.ghClient.ListIssues(ctx, state, 20)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage(gh.FormatIssueList(issues))
		return nil
	case "view", "show":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage: /issue view <number>")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		out, err := m.ghClient.ViewIssue(ctx, n)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("Issue:\n" + out)
		return nil
	case "create", "new":
		if len(rest) == 0 {
			m.chat.AddSystemMessage("Usage: /issue create <title> [| body]")
			return nil
		}
		joined := strings.Join(rest, " ")
		title, body := joined, ""
		if i := strings.Index(joined, " | "); i >= 0 {
			title = strings.TrimSpace(joined[:i])
			body = strings.TrimSpace(joined[i+3:])
		}
		out, err := m.ghClient.CreateIssue(ctx, title, body, nil)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Issue created\n" + out)
		m.toast = components.NewToast("Issue created", "success", 3*time.Second)
		return nil
	case "comment":
		if len(rest) < 2 {
			m.chat.AddSystemMessage("Usage: /issue comment <number> <body…>")
			return nil
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(rest[0], "#"))
		body := strings.Join(rest[1:], " ")
		out, err := m.ghClient.CommentOnIssue(ctx, n, body)
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.chat.AddSystemMessage("✓ Issue comment\n" + out)
		return nil
	default:
		m.chat.AddSystemMessage("Unknown /issue subcommand. " + githubHelpText())
		return nil
	}
}

func githubHelpText() string {
	return `GitHub integration (gh CLI or GITHUB_TOKEN)

AUTH & REPO
  /gh auth              Auth status + user + repo slug
  /gh repo              Repository metadata
  /gh log               Recent commits
  /gh branch [name]     Show or create branch

SYNC
  /push                 git push -u origin HEAD
  /pull                 git pull
  /commit [message]     Stage all + commit

PULL REQUESTS
  /pr list [state]      List PRs (open|closed|merged|all)
  /pr view [number]     View PR (current branch if omitted)
  /pr create <title> [| body]
  /pr merge <n> [squash|merge|rebase]
  /pr checks [n]        CI status
  /pr diff [n]          Full PR diff
  /pr comment <n> body  Comment on PR
  /pr review <n> u1,u2  Request reviewers
  /pr commits [n]       Commits on PR
  /pr babysit [n] [--fix]  Poll CI until green; --fix runs agent on failure

ISSUES
  /issue list [state]
  /issue view <n>
  /issue create <title> [| body]
  /issue comment <n> body

AGENT TOOLS
  search_replace · apply_patch · github (babysit, pr_diff, …)

Setup:  gh auth login   OR   export GITHUB_TOKEN=ghp_...`
}

func parseGitStatus(status string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		line = strings.TrimSpace(line)
		if len(line) < 4 {
			continue
		}
		// format from git.Status: "  XY  path"
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		code := fields[0]
		path := fields[len(fields)-1]
		out[path] = code
	}
	return out
}

// ────────────────────────────────────────────────────────────
// Messages & pumps
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

type DiffUpdateMsg struct {
	Content string
	Pending bool
}

type ContextUpdateMsg struct {
	Files     []string
	Refresh   bool
	GitStatus map[string]string
}

type ToastMsg struct {
	Text string
	Kind string
}

// GitHubStatusMsg carries async auth discovery results.
type GitHubStatusMsg struct {
	User string
	Repo string
	OK   bool
	Err  string
}

// BabysitDoneMsg is returned when /pr babysit polling finishes.
type BabysitDoneMsg struct {
	Status gh.CheckStatus
	Err    error
	PR     int
	Fix    bool
}

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
			Text: tok.Text, Done: tok.Done,
			InputTokens: tok.InputTokens, OutputTokens: tok.OutputTokens,
			Error: tok.Error,
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

func modeString(m Mode) string {
	switch m {
	case ModeNormal:
		return "NORMAL"
	case ModeInsert:
		return "INSERT"
	case ModeCommand:
		return "COMMAND"
	case ModePalette:
		return "COMMAND"
	case ModeReview:
		return "REVIEW"
	case ModeFilePick:
		return "INSERT"
	}
	return "?"
}

var slashCommands = []string{
	"/act", "/read", "/ls", "/grep", "/run", "/explain", "/fix",
	"/status", "/commit", "/push", "/pull", "/pr", "/issue", "/gh",
	"/provider", "/model", "/mode", "/cost", "/budget", "/rules", "/index",
	"/sessions", "/undo", "/clear", "/help", "/about", "/quit",
}

func autocomplete(input string) string {
	for _, cmd := range slashCommands {
		if strings.HasPrefix(cmd, input) {
			return cmd + " "
		}
	}
	return ""
}

func helpText() string {
	return `CodeForge TUI v0.6.0  ·  NanoMind 2026  ·  Tier-2 Intelligence

` + keymap.FullHelp() + `

PROJECT INTELLIGENCE
  /rules     Show AGENTS.md / project rules
  /index     Codebase index stats (codebase_search tool)
  /budget    Session cost vs budget.max_cost_usd

GITHUB & SHIP
  /gh · /pr · /issue · /pr babysit [--fix]

AGENT TOOLS
  research · codebase_search · diagnostics · fetch_url
  search_replace · apply_patch · github · mcp_*`
}

func aboutText() string {
	return `CodeForge TUI v0.6.0
Created by NanoMind — 2026 — Apache 2.0

Tier-2: AGENTS.md rules · codebase index · diagnostics · fetch_url
        research sub-agent · MCP servers · cost budget · secret redaction
Stack: Go · Bubble Tea · Glamour · multi-provider · gh
Design: Terminal Glass / Aurora Dark

"Terminal AI coding companion — open, modular, vendor-neutral — and it feels like the future."
                        — NanoMind, 2026`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
