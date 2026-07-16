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
	"github.com/codeforge/tui/internal/bgtask"
	"github.com/codeforge/tui/internal/checkpoint"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/git"
	gh "github.com/codeforge/tui/internal/github"
	"github.com/codeforge/tui/internal/hooks"
	"github.com/codeforge/tui/internal/index"
	"github.com/codeforge/tui/internal/keymap"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/personas"
	"github.com/codeforge/tui/internal/sandbox"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/skills"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/todos"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/tui/sessionpicker"
	"github.com/codeforge/tui/internal/tui/slashmenu"
	"github.com/codeforge/tui/internal/tui/themepicker"
	"github.com/codeforge/tui/internal/ui/askuser"
	"github.com/codeforge/tui/internal/ui/blockview"
	"github.com/codeforge/tui/internal/ui/clipboard"
	"github.com/codeforge/tui/internal/ui/components"
	"github.com/codeforge/tui/internal/ui/filepicker"
	"github.com/codeforge/tui/internal/ui/markdown"
	"github.com/codeforge/tui/internal/ui/palette"
	"github.com/codeforge/tui/internal/ui/permask"
	"github.com/codeforge/tui/internal/ui/planreview"
	"github.com/codeforge/tui/internal/ui/review"
	"github.com/codeforge/tui/internal/ui/settings"
)

// Grok-style layout: full-width scrollback + bottom prompt.
// Side drawers (diff/files) toggle with Ctrl+B — off by default like Grok.

type Model struct {
	cfg         *config.Config
	providerReg *provider.Registry
	toolReg     *tool.Registry
	gitRepo     *git.Repo
	workdir     string
	keys        keymap.Map

	width  int
	height int
	// focusPrompt true = Grok "prompt focused"; false = scrollback focused
	focusPrompt bool
	mode        Mode
	// sessionMode: BUILD (staged) → DESIGN (plan-only) → YOLO (always-approve)
	sessionMode tool.SessionMode
	showPanels  bool // side drawers Diff+Files
	activePane  Pane // when panels on

	chat    ChatModel
	diff    DiffModel
	context ContextModel
	status  StatusBarModel
	command CommandModel
	palette palette.Model
	picker  filepicker.Model
	review  review.Model
	planUI   planreview.Model
	slash    slashmenu.Model
	themes   themepicker.Model
	sessions sessionpicker.Model
	rewinds  sessionpicker.RewindModel
	blockV   blockview.Model
	settings settings.Model
	toast    components.Toast

	streamCh <-chan provider.StreamToken
	agentCh  <-chan agent.Event

	session     *session.Session
	ghClient    *gh.Client
	quitting    bool
	startTime   time.Time
	totalCost   float64
	totalTokens int

	// Esc double-press (Grok: clear prompt / rewind)
	lastEsc time.Time
	// Ctrl+C: first clears draft / cancels turn; second quits when idle
	ctrlCArmed bool

	// vimMode: j/k/h/l only when scrollback focused (Grok [ui].vim_mode)
	vimMode bool

	// motion
	borderPhase float64
	lastTokenAt time.Time
	tokenWindow int

	// Phase 6 permissions
	perm     *permission.Engine
	hooks    *hooks.Runner
	permAsk  permask.Model
	permReq  chan permAskRequest // agent → UI
	permWait *permAskRequest     // active request awaiting y/n

	// Phase G2: agent ask_user_question option picker
	userAsk askuser.Model
}

// permAskRequest is a blocking permission prompt from the agent goroutine.
type permAskRequest struct {
	Tool, Input, Reason string
	Dangerous           bool
	Reply               chan permAskReply
}

type permAskReply struct {
	Allow  bool
	Always bool
}

// PermAskMsg opens the permission modal (from listenPermAsk cmd).
type PermAskMsg struct {
	Req permAskRequest
}

// AutoThemeTickMsg re-checks system appearance when theme=auto.
type AutoThemeTickMsg struct{}

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
	ModeThemePick
	ModeSessionPick
	ModeRewindPick
	ModePlanReview
	ModePermAsk
	ModeBlockView
	ModeSettings
	ModeAskUser
)

func New(cfg *config.Config, provReg *provider.Registry, toolReg *tool.Registry, repo *git.Repo, workdir string) Model {
	sess := session.New(provReg.CurrentName(), "", workdir)
	if cur, err := provReg.Current(); err == nil {
		sess.Model = cur.Model()
	}
	ghc := gh.New(workdir)
	chat := NewChatModel(provReg, toolReg, repo, workdir)

	// Phase 6: permissions + hooks
	permEng := permission.FromConfig(cfg, workdir)
	hookRunner := hooks.Load(workdir)
	askCh := make(chan permAskRequest, 1)
	permEng.Ask = func(ctx context.Context, toolName, input, reason string, dangerous bool) (bool, bool, error) {
		reply := make(chan permAskReply, 1)
		req := permAskRequest{
			Tool: toolName, Input: input, Reason: reason,
			Dangerous: dangerous, Reply: reply,
		}
		select {
		case askCh <- req:
		case <-ctx.Done():
			return false, false, ctx.Err()
		}
		select {
		case r := <-reply:
			return r.Allow, r.Always, nil
		case <-ctx.Done():
			return false, false, ctx.Err()
		}
	}
	chat.Auth = permEng
	// Project rules
	rb := rules.Get()
	if rb != nil && rb.Text != "" {
		chat.SetRules(rb.Text)
	}
	// Grok simple mode: start with prompt focused (ready to type)
	chat.mode = ModeInsert
	chat.FocusInput()
	vim := false
	if cfg != nil {
		vim = cfg.UI.VimMode
		if cfg.UI.CompactMode {
			theme.SetCompact(true)
		}
	}
	m := Model{
		cfg:         cfg,
		providerReg: provReg,
		toolReg:     toolReg,
		gitRepo:     repo,
		workdir:     workdir,
		keys:        keymap.Default(),
		focusPrompt: true,
		mode:        ModeInsert,
		sessionMode: tool.SessionBuild,
		showPanels:  false,
		activePane:  PaneChat,
		startTime:   time.Now(),
		chat:        chat,
		diff:        NewDiffModel(),
		context:     NewContextModel(workdir),
		status:      NewStatusBarModel(),
		command:     NewCommandModel(),
		palette:     palette.New(),
		picker:      filepicker.New(workdir),
		review:      review.New(),
		planUI:      planreview.New(),
		slash:       slashmenu.New(),
		themes:      themepicker.New(),
		sessions:    sessionpicker.New(),
		rewinds:     sessionpicker.NewRewind(),
		blockV:      blockview.New(),
		settings:    settings.New(),
		session:     sess,
		ghClient:    ghc,
		vimMode:     vim,
		perm:        permEng,
		hooks:       hookRunner,
		permAsk:     permask.New(),
		permReq:     askCh,
		userAsk:     askuser.New(),
	}
	// Wire plan.md path + BUILD write mode (staged)
	m.syncWriteMode()
	// Align permission mode with session YOLO if always_approve
	if permEng != nil && permEng.GetMode() == permission.ModeAlwaysApprove {
		m.sessionMode = tool.SessionYolo
		m.syncWriteMode()
	}
	if rb != nil && len(rb.Paths) > 0 {
		m.chat.AddSystemMessage(rb.Summary())
	}
	if hookRunner != nil && hookRunner.Count() > 0 {
		m.chat.AddSystemMessage(fmt.Sprintf("Hooks: %d loaded", hookRunner.Count()))
	}
	return m
}

func listenPermAsk(ch <-chan permAskRequest) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		req, ok := <-ch
		if !ok {
			return nil
		}
		return PermAskMsg{Req: req}
	}
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.chat.Init(),
		m.context.Init(),
		spinnerTick(),
		listenPermAsk(m.permReq),
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
	}
	if theme.IsAuto() {
		cmds = append(cmds, autoThemeTick())
	}
	return tea.Batch(cmds...)
}

func autoThemeTick() tea.Cmd {
	return tea.Tick(theme.AutoPollInterval, func(t time.Time) tea.Msg {
		return AutoThemeTickMsg{}
	})
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

	case AutoThemeTickMsg:
		if theme.IsAuto() {
			before := theme.DisplayName()
			theme.ResolveAuto()
			if theme.DisplayName() != before {
				m.onThemeApplied()
			}
			cmds = append(cmds, autoThemeTick())
		}

	case PermAskMsg:
		m.permWait = &msg.Req
		m.permAsk.Open(msg.Req.Tool, msg.Req.Input, msg.Req.Reason, msg.Req.Dangerous)
		m.mode = ModePermAsk
		m.focusPrompt = false
		m.chat.BlurInput()
		// keep listening for further asks after this one resolves
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.recalcSizes()
		m.review.Width = msg.Width
		m.review.Height = msg.Height - 2

	case tea.KeyMsg:
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

func isImmediateSlash(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	switch cmd {
	case "/help", "/about", "/cost", "/budget", "/rules", "/index",
		"/status", "/clear", "/quit", "/theme", "/compact-mode", "/vim-mode",
		"/sessions", "/resume", "/new", "/fork", "/rewind", "/compact",
		"/context", "/session-info", "/mode", "/plan", "/view-plan",
		"/permissions", "/sandbox", "/hooks", "/todos", "/tasks", "/settings", "/copy",
		"/memory", "/skills", "/personas", "/subagents", "/undo", "/push", "/pull":
		return true
	default:
		return false
	}
}

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
		{Key: "sandbox", Value: string(sandbox.Global().Profile), Hint: "/sandbox"},
		{Key: "provider", Value: m.providerReg.CurrentName(), Hint: modelName},
		{Key: "todos", Value: todos.Global.Badge(), Hint: "/todos"},
		{Key: "bg_tasks", Value: fmt.Sprintf("%d running", bgtask.Global.RunningCount()), Hint: "/tasks"},
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
			m.chat.AddSystemMessage(todos.Global.Render())
			m.mode = ModeInsert
			m.focusPrompt = true
			m.chat.FocusInput()
			return m, nil
		case "bg_tasks":
			m.settings.Close()
			m.chat.AddSystemMessage(bgtask.Global.Summary())
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

func (m *Model) handleAgentEvent(ev agent.Event) []tea.Cmd {
	var cmds []tea.Cmd
	nc, c := m.chat.Update(AgentEventMsg{Ev: ev})
	m.chat = nc.(ChatModel)
	if c != nil {
		cmds = append(cmds, c)
	}
	// Design-plan tool signals (enter/exit/write)
	if sig := tool.ConsumePlanSignal(); sig != nil {
		switch sig.Kind {
		case "enter_plan_mode":
			if m.sessionMode != tool.SessionDesign {
				m.setSessionMode(tool.SessionDesign)
			}
			m.chat.AddSystemMessage("◈ Agent requested DESIGN: " + sig.Message)
		case "exit_plan_mode":
			m.openPlanReview(sig.Message)
		case "plan_written":
			m.chat.AddSystemMessage("Plan updated → " + sig.Message)
		}
	}

	if ev.Kind == agent.EventDone {
		m.accTokens(ev.InputTokens, ev.OutputTokens)
		// If agent called exit_plan_mode mid-turn, approval already open
		if m.mode == ModePlanReview {
			cmds = append(cmds, m.persistSessionCmd())
			return cmds
		}
		// BUILD mode: open patch review if pending
		if sw := m.toolReg.GetStagedWriter(); sw != nil && sw.HasPending() {
			patches := sw.Pending()
			m.review.Open(patches)
			m.mode = ModeReview
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
	if ev.Kind == agent.EventToolResult && (ev.ToolName == "ask_user_question" || ev.ToolName == "ask_user") {
		if q := tool.ConsumePendingAsk(); q != nil {
			msg := "❓ " + q.Question
			if len(q.Options) > 0 {
				msg += "\n"
				for i, o := range q.Options {
					msg += fmt.Sprintf("  %d) %s\n", i+1, o)
				}
			}
			m.chat.AddSystemMessage(msg)
			m.toast = components.NewToast("Agent is waiting for your answer", "info", 4*time.Second)
			// Interactive option modal (G2 polish)
			m.userAsk.Open(q.Question, q.Options)
			m.mode = ModeAskUser
		}
	}
	if ev.Kind == agent.EventToolResult && ev.ToolDiff != "" {
		nd, dc := m.diff.Update(DiffUpdateMsg{Content: ev.ToolDiff, Pending: m.sessionMode == tool.SessionBuild})
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
				// YOLO mode: checkpoint immediately
				if m.sessionMode == tool.SessionYolo {
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
	// Grok layout padding matrix (full / compact / minimal)
	footerH := theme.FooterHeight()
	promptH := theme.PromptHeight()
	if m.toast.Alive() {
		footerH++
	}
	mainH := m.height - footerH - promptH
	if mainH < 6 {
		mainH = 6
	}
	if m.showPanels && m.width >= 100 {
		chatW := m.width * 58 / 100
		sideW := (m.width - chatW - 4) / 2
		if sideW < 14 {
			sideW = 14
		}
		m.chat.SetSize(chatW, mainH)
		m.diff.SetSize(sideW, mainH)
		m.context.SetSize(sideW, mainH)
	} else {
		// Full-width scrollback (Grok default)
		m.chat.SetSize(m.width, mainH)
		m.diff.SetSize(m.width, mainH)
		m.context.SetSize(m.width, mainH)
	}
	m.status.SetSize(m.width)
	m.command.SetSize(m.width, mainH)
	m.palette.SetSize(m.width, m.height)
	m.picker.Width = min(50, m.width-4)
}

func (m *Model) syncStatus() {
	// Grok focus labels
	if m.focusPrompt || m.mode == ModeInsert {
		m.status.Mode = "PROMPT"
	} else {
		m.status.Mode = "SCROLL"
	}
	if m.mode == ModePalette {
		m.status.Mode = "PALETTE"
	}
	if m.mode == ModeReview {
		m.status.Mode = "REVIEW"
	}
	m.status.Provider = m.providerReg.CurrentName()
	if cur, err := m.providerReg.Current(); err == nil {
		m.status.ModelName = cur.Model()
	}
	m.status.Tokens = m.totalTokens
	m.status.Cost = m.totalCost
	m.status.Streaming = m.chat.streaming
	m.status.ThemeName = theme.Name()
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
	m.status.AgentMode = m.sessionMode.Label()
	m.status.TodoBadge = todos.Global.Badge()
	m.status.BgTasks = bgtask.Global.RunningCount()
	m.status.Sandbox = sandbox.Global().Label()
	// show running subagents alongside bg shell tasks
	if n := tool.SubJobs.RunningCount(); n > 0 {
		m.status.BgTasks += n
	}
	if m.gitRepo != nil {
		if branch, err := m.gitRepo.Branch(); err == nil {
			m.status.Branch = branch
		}
	}
	m.status.Workdir = m.workdir
	// Keep chat mode aligned with focus
	if m.focusPrompt {
		m.chat.mode = ModeInsert
	} else {
		m.chat.mode = ModeNormal
	}
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
		return lipgloss.NewStyle().Foreground(theme.Current().AccentUser).Render(
			"Goodbye — CodeForge · Grok-style TUI\n")
	}
	if m.width == 0 {
		return "Starting CodeForge…\n"
	}

	// ── Review fullscreen ────────────────────────────
	if m.mode == ModeReview {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.review.View(),
			m.status.ViewFooter(),
		)
	}
	// ── Design plan approval fullscreen ──────────────
	if m.mode == ModePlanReview {
		m.planUI.Width = m.width
		m.planUI.Height = m.height - 1
		return lipgloss.JoinVertical(lipgloss.Left,
			m.planUI.View(),
			m.status.ViewFooter(),
		)
	}
	// ── Permission ask modal ──────────────────────────
	if m.mode == ModePermAsk {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.chat.ViewScrollback(),
			m.permAsk.View(),
			m.status.ViewFooter(),
		)
	}
	// ── Agent ask_user_question modal ─────────────────
	if m.mode == ModeAskUser {
		return lipgloss.JoinVertical(lipgloss.Left,
			m.chat.ViewScrollback(),
			m.userAsk.View(),
			m.status.ViewFooter(),
		)
	}
	// ── Fullscreen block viewer ───────────────────────
	if m.mode == ModeBlockView {
		m.blockV.Width = m.width
		m.blockV.Height = m.height - 1
		return lipgloss.JoinVertical(lipgloss.Left,
			m.blockV.View(),
			m.status.ViewFooter(),
		)
	}
	// ── Settings overlay (composite over main) ────────
	// rendered with other overlays below

	// ── Main: Grok scrollback (optional side drawers) ─
	scroll := m.chat.ViewScrollback()
	var main string
	if m.showPanels && m.width >= 100 {
		main = lipgloss.JoinHorizontal(lipgloss.Top,
			scroll,
			theme.PaneBorder(m.activePane == PaneDiff, 0, 0).Render(m.diff.View()),
			theme.PaneBorder(m.activePane == PaneContext, 0, 0).Render(m.context.View()),
		)
	} else {
		main = scroll
	}

	parts := []string{}
	if m.toast.Alive() {
		parts = append(parts, m.toast.View(m.width))
	}
	parts = append(parts, main)

	// Overlays above prompt
	switch m.mode {
	case ModeCommand:
		parts = append(parts, m.command.View())
	case ModePalette:
		parts = append(parts, m.palette.View())
	case ModeFilePick:
		parts = append(parts, m.picker.View())
	case ModeThemePick:
		m.themes.Width = m.width
		parts = append(parts, m.themes.View())
	case ModeSettings:
		m.settings.Width = m.width
		parts = append(parts, m.settings.View())
	case ModeSessionPick:
		m.sessions.Width = m.width
		m.sessions.Height = m.height
		parts = append(parts, m.sessions.View())
	case ModeRewindPick:
		m.rewinds.Width = m.width
		parts = append(parts, m.rewinds.View())
	}

	// Slash menu floats above composer
	if m.slash.Active && m.focusPrompt {
		m.slash.Width = m.width
		parts = append(parts, m.slash.View())
	}
	// Composer + Grok footer
	parts = append(parts, m.chat.ViewPrompt(m.focusPrompt))
	if !theme.MinimalMode() {
		parts = append(parts, m.status.ViewFooter())
		if !theme.CompactMode() {
			parts = append(parts, m.status.ViewBottom())
		}
	} else {
		// minimal: one slim status line only
		parts = append(parts, m.status.ViewFooter())
	}
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// updateThemePicker handles /theme interactive list (live preview).
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
			m.chat.AddSystemMessage(fmt.Sprintf(
				"Session mode: %s\n  %s\n\n  /mode build   — staged writes (default)\n  /mode design  — plan.md only (DESIGN)\n  /mode yolo    — always-approve writes\n  Shift+Tab     — cycle BUILD → DESIGN → YOLO\n  /plan         — enter DESIGN + optional task",
				m.sessionMode.Label(), m.sessionMode.Description(),
			))
		} else {
			if sm, ok := tool.ParseSessionMode(args[0]); ok {
				m.setSessionMode(sm)
			} else {
				// legacy aliases handled by ParseSessionMode; leftover:
				m.chat.AddSystemMessage("Use: /mode build | design | yolo  (aliases: plan→design, act→yolo)")
			}
		}

	case "plan":
		// Enter DESIGN; optional arg starts an agent plan turn
		m.setSessionMode(tool.SessionDesign)
		if argStr != "" {
			task := "Design a plan (DESIGN mode) for: " + argStr +
				"\n\nExplore the codebase with read/search tools. Write the plan via write_plan. " +
				"Then call exit_plan_mode. Do NOT edit project source files."
			return m.chat.SubmitAgent(task)
		}
		m.chat.AddSystemMessage("◈ DESIGN mode on. Describe the task and press Enter, or:\n  /plan <description>  — start planning turn\n  /view-plan            — open current plan")

	case "view-plan", "show-plan", "plan-view":
		m.openPlanReview("View plan")
		// viewing only — if user approves from here, still OK

	case "permissions", "perms":
		if m.perm == nil {
			m.chat.AddSystemMessage("Permission engine not available")
			return nil
		}
		if len(args) == 0 {
			m.chat.AddSystemMessage(m.perm.Summary() + "\n  /permissions mode <default|always_approve|dont_ask|plan>\n  /permissions allow <tool> [pattern]\n  /permissions deny <tool> [pattern]\n  /permissions clear-remember")
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "mode":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Current: " + string(m.perm.GetMode()))
				return nil
			}
			m.perm.SetMode(permission.ParseMode(args[1]))
			m.chat.AddSystemMessage("Permission mode → " + string(m.perm.GetMode()))
		case "allow":
			toolN := "*"
			pat := "*"
			if len(args) >= 2 {
				toolN = args[1]
			}
			if len(args) >= 3 {
				pat = strings.Join(args[2:], " ")
			}
			m.perm.AddRule(permission.Rule{Tool: toolN, Pattern: pat, Effect: permission.EffectAllow})
			m.chat.AddSystemMessage(fmt.Sprintf("Added allow rule: %s %q", toolN, pat))
		case "deny":
			toolN := "*"
			pat := "*"
			if len(args) >= 2 {
				toolN = args[1]
			}
			if len(args) >= 3 {
				pat = strings.Join(args[2:], " ")
			}
			m.perm.AddRule(permission.Rule{Tool: toolN, Pattern: pat, Effect: permission.EffectDeny})
			m.chat.AddSystemMessage(fmt.Sprintf("Added deny rule: %s %q", toolN, pat))
		case "ask":
			toolN := "run_command"
			pat := "*"
			if len(args) >= 2 {
				toolN = args[1]
			}
			if len(args) >= 3 {
				pat = strings.Join(args[2:], " ")
			}
			m.perm.AddRule(permission.Rule{Tool: toolN, Pattern: pat, Effect: permission.EffectAsk})
			m.chat.AddSystemMessage(fmt.Sprintf("Added ask rule: %s %q", toolN, pat))
		case "clear-remember", "clear":
			m.perm.ClearRemembered()
			m.chat.AddSystemMessage("Cleared remembered grants")
		default:
			m.chat.AddSystemMessage("Usage: /permissions [mode|allow|deny|ask|clear-remember]")
		}

	case "hooks":
		if m.hooks == nil {
			m.chat.AddSystemMessage("No hooks runner")
			return nil
		}
		m.chat.AddSystemMessage(m.hooks.Summary())

	case "sandbox", "sbx":
		eng := sandbox.Global()
		if len(args) == 0 {
			bwrap := "no"
			if sandbox.HasBubblewrap() {
				bwrap = "yes"
			}
			m.chat.AddSystemMessage(fmt.Sprintf(
				"%s\nbubblewrap: %s\n\n  /sandbox off         — unrestricted\n  /sandbox workspace   — write CWD + ~/.codeforge + tmp (recommended)\n  /sandbox read-only   — no project writes; net blocked for shell\n  /sandbox strict      — read CWD+system only; net blocked\n  /sandbox devbox      — write almost everywhere except /data\n\nEnv: CODEFORGE_SANDBOX · flag: --sandbox · docs/SANDBOX.md",
				eng.Summary(), bwrap,
			))
			return nil
		}
		p, ok := sandbox.ParseProfile(args[0])
		if !ok {
			m.chat.AddSystemMessage("Unknown profile. Use: off | workspace | read-only | strict | devbox")
			return nil
		}
		// Preserve deny list from previous engine
		deny := append([]string(nil), eng.Deny...)
		ne := sandbox.Ensure(p, m.workdir)
		ne.Deny = deny
		sandbox.LogEvent("switch", map[string]any{"profile": string(p), "backend": string(ne.Backend)})
		m.chat.AddSystemMessage("✓ " + ne.Summary())
		m.toast = components.NewToast(ne.Summary(), "info", 3*time.Second)

	case "todos", "todo":
		if len(args) == 0 {
			m.chat.AddSystemMessage(todos.Global.Render())
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "add":
			text := strings.Join(args[1:], " ")
			if text == "" {
				m.chat.AddSystemMessage("Usage: /todos add <text>")
				return nil
			}
			it := todos.Global.Add(text)
			m.chat.AddSystemMessage(fmt.Sprintf("Added %s: %s", it.ID, it.Content))
		case "done", "complete":
			if len(args) < 2 || !todos.Global.SetStatus(args[1], todos.Completed) {
				m.chat.AddSystemMessage("Usage: /todos done <id>")
				return nil
			}
			m.chat.AddSystemMessage("Completed " + args[1])
		case "progress", "start":
			if len(args) < 2 || !todos.Global.SetStatus(args[1], todos.InProgress) {
				m.chat.AddSystemMessage("Usage: /todos progress <id>")
				return nil
			}
			m.chat.AddSystemMessage("In progress " + args[1])
		case "clear":
			todos.Global.Clear()
			m.chat.AddSystemMessage("Todos cleared")
		default:
			m.chat.AddSystemMessage("Usage: /todos [add|done|progress|clear]")
		}

	case "subagents", "subagent", "subs":
		if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls") {
			m.chat.AddSystemMessage(tool.SubJobs.Summary())
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "show", "view", "get", "output":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /subagents show <id>")
				return nil
			}
			j, ok := tool.SubJobs.Get(args[1])
			if !ok {
				m.chat.AddSystemMessage("Unknown subagent: " + args[1])
				return nil
			}
			m.chat.AddSystemMessage(tool.FormatJobOutput(j))
		case "cancel", "kill", "stop":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /subagents cancel <id>")
				return nil
			}
			if err := tool.SubJobs.Cancel(args[1]); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("Cancel signal sent to " + args[1])
				m.toast = components.NewToast("Subagent cancel "+args[1], "info", 2*time.Second)
			}
		default:
			// treat as id
			j, ok := tool.SubJobs.Get(args[0])
			if !ok {
				m.chat.AddSystemMessage("Usage: /subagents [list|show <id>|cancel <id>]")
				return nil
			}
			m.chat.AddSystemMessage(tool.FormatJobOutput(j))
		}

	case "personas", "persona":
		if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls") {
			m.chat.AddSystemMessage(personas.Global().RenderList())
			return nil
		}
		if strings.EqualFold(args[0], "reload") {
			cfgP := map[string]personas.Persona{}
			var extra []string
			if m.cfg != nil {
				for name, sp := range m.cfg.Subagents.Personas {
					cfgP[name] = personas.Persona{
						Name: name, Description: sp.Description, Instructions: sp.Instructions,
						InstructionsFile: sp.InstructionsFile, Model: sp.Model, DefaultIsolation: sp.DefaultIsolation,
					}
				}
				extra = m.cfg.Subagents.ExtraDirs
			}
			reg := personas.Load(personas.Options{WorkDir: m.workdir, ConfigPersonas: cfgP, ExtraDirs: extra})
			m.chat.AddSystemMessage("✓ Reloaded " + reg.Summary())
			return nil
		}
		if p, ok := personas.Global().Get(args[0]); ok {
			body := p.Resolved
			if body == "" {
				body = p.Instructions
			}
			if len(body) > 2500 {
				body = body[:2500] + "\n…"
			}
			m.chat.AddSystemMessage(fmt.Sprintf("Persona %s\n%s\n\n%s\n\nUse: spawn_subagent persona=%s",
				p.Name, p.Description, body, p.Name))
			return nil
		}
		m.chat.AddSystemMessage("Unknown persona: " + args[0])

	case "skills", "skill":
		// /skills [list|reload|<name>]
		if len(args) == 0 || strings.EqualFold(args[0], "list") || strings.EqualFold(args[0], "ls") {
			m.chat.AddSystemMessage(skills.Global().RenderList())
			return nil
		}
		if strings.EqualFold(args[0], "reload") || strings.EqualFold(args[0], "refresh") {
			opt := skills.Options{WorkDir: m.workdir, CompatClaude: true, CompatCursor: true}
			if m.cfg != nil {
				opt.ExtraPaths = m.cfg.Skills.Paths
				opt.Ignore = m.cfg.Skills.Ignore
				opt.Disabled = m.cfg.Skills.Disabled
				opt.CompatClaude = m.cfg.SkillsCompatClaude()
				opt.CompatCursor = m.cfg.SkillsCompatCursor()
			}
			reg := skills.Load(opt)
			m.slash.RefreshSkills()
			m.chat.AddSystemMessage("✓ Reloaded " + reg.Summary())
			m.toast = components.NewToast(reg.Summary(), "info", 2*time.Second)
			return nil
		}
		// show one skill
		name := args[0]
		if sk, ok := skills.Global().Get(name); ok {
			body := sk.Body
			if len(body) > 3000 {
				body = body[:3000] + "\n… (truncated)"
			}
			m.chat.AddSystemMessage(fmt.Sprintf("Skill /%s [%s]\n%s\n\n%s\n\nRun: /%s [args]",
				sk.Name, sk.Source, sk.Description, body, sk.Name))
			return nil
		}
		m.chat.AddSystemMessage("Unknown skill: " + name + "\nUse /skills to list.")

	case "memory", "mem":
		// /memory [list|add <text>|search <query>]
		if len(args) == 0 {
			m.chat.AddSystemMessage("Memory store (~/.codeforge/memory/):\n  /memory list          — recent notes\n  /memory add <text>    — store a note\n  /memory search <q>    — keyword search\n\nAgents use memory_search / memory_write tools.")
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "list", "ls", "show":
			notes, err := tool.ListMemoryRecent(15)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			if len(notes) == 0 {
				m.chat.AddSystemMessage("(no memory notes yet — try /memory add …)")
				return nil
			}
			var b strings.Builder
			b.WriteString("Recent memory notes:\n")
			for i, n := range notes {
				fmt.Fprintf(&b, "  %d. %s\n", i+1, n)
			}
			m.chat.AddSystemMessage(b.String())
		case "add", "write", "save":
			text := strings.Join(args[1:], " ")
			if text == "" {
				m.chat.AddSystemMessage("Usage: /memory add <text>")
				return nil
			}
			if err := tool.AppendMemory(text, ""); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage("✓ Memory note stored")
			m.toast = components.NewToast("Memory saved", "success", 2*time.Second)
		case "search", "find", "s":
			q := strings.Join(args[1:], " ")
			if q == "" {
				m.chat.AddSystemMessage("Usage: /memory search <query>")
				return nil
			}
			ms := &tool.MemorySearch{}
			res := ms.Execute([]byte(fmt.Sprintf(`{"query":%q,"limit":10}`, q)))
			if res.Error != "" {
				m.chat.AddSystemMessage("⚠ " + res.Error)
				return nil
			}
			m.chat.AddSystemMessage(res.Output)
		default:
			// bare text after /memory → treat as add
			if err := tool.AppendMemory(argStr, ""); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			m.chat.AddSystemMessage("✓ Memory note stored (implicit add)")
		}

	case "tasks", "bg":
		if len(args) == 0 {
			m.chat.AddSystemMessage(bgtask.Global.Summary())
			return nil
		}
		switch strings.ToLower(args[0]) {
		case "cancel", "kill", "stop":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /tasks cancel <id>")
				return nil
			}
			if err := bgtask.Global.Cancel(args[1]); err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
			} else {
				m.chat.AddSystemMessage("Cancelled " + args[1])
				m.toast = components.NewToast("Task cancelled", "info", 2*time.Second)
			}
		case "show", "view", "log":
			if len(args) < 2 {
				m.chat.AddSystemMessage("Usage: /tasks show <id>")
				return nil
			}
			t, ok := bgtask.Global.Get(args[1])
			if !ok {
				m.chat.AddSystemMessage("Task not found")
				return nil
			}
			m.chat.AddSystemMessage(fmt.Sprintf("[%s] %s\n%s\n\n%s", t.Status, t.ID, t.Command, t.Output))
		default:
			m.chat.AddSystemMessage(bgtask.Global.Summary())
		}

	case "settings", "config":
		m.openSettings()

	case "copy":
		// /copy [meta]
		meta := len(args) > 0 && (args[0] == "meta" || args[0] == "Y")
		m.copySelectedBlock(meta)

	case "sessions", "dashboard":
		// list text view (picker is /resume)
		if len(args) == 0 {
			list, err := session.ListForWorkdir(m.workdir, 15)
			if err != nil {
				m.chat.AddSystemMessage("⚠ " + err.Error())
				return nil
			}
			if len(list) == 0 {
				m.chat.AddSystemMessage("No saved sessions. Use /resume or just chat.")
				return nil
			}
			var sb strings.Builder
			sb.WriteString("Sessions (/resume picker · /sessions <id>):\n")
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
			m.applySession(s)
		}

	case "resume":
		if len(args) == 0 {
			m.openSessionPicker()
			return nil
		}
		s, err := session.Load(args[0])
		if err != nil {
			m.chat.AddSystemMessage("⚠ " + err.Error())
			return nil
		}
		m.applySession(s)

	case "new":
		m.startNewSession()

	case "fork":
		if m.session == nil {
			m.chat.AddSystemMessage("No session to fork")
			return nil
		}
		m.session.Messages = m.chat.messages
		m.session.TotalCost = m.totalCost
		m.session.Tokens = m.totalTokens
		child, err := m.session.Fork()
		if err != nil {
			m.chat.AddSystemMessage("⚠ fork: " + err.Error())
			return nil
		}
		// optional directive as first user note
		if argStr != "" {
			child.Messages = append(child.Messages, provider.Message{
				Role: provider.RoleUser, Content: argStr,
			})
			_ = child.Save()
		}
		m.applySession(child)
		m.chat.AddSystemMessage("Forked → " + child.ID + " (parent " + child.ParentID + ")")

	case "rewind":
		if len(args) == 0 {
			m.openRewindPicker()
			return nil
		}
		// /rewind last — last point
		pts, err := m.session.LoadRewindPoints()
		if err != nil || len(pts) == 0 {
			pts = synthesizeRewindPoints(m.chat.messages)
		}
		if len(pts) == 0 {
			m.chat.AddSystemMessage("No rewind points")
			return nil
		}
		if strings.EqualFold(args[0], "last") {
			m.applyRewind(pts[len(pts)-1])
			return nil
		}
		// by index 1-based from newest
		if n, err := strconv.Atoi(args[0]); err == nil && n > 0 {
			// newest first numbering
			idx := len(pts) - n
			if idx >= 0 && idx < len(pts) {
				m.applyRewind(pts[idx])
				return nil
			}
		}
		m.chat.AddSystemMessage("Usage: /rewind | /rewind last | /rewind <n>")

	case "compact":
		// conversation compact (not compact-mode UI)
		if m.session == nil {
			m.chat.AddSystemMessage("No session")
			return nil
		}
		m.session.Messages = m.chat.messages
		res, err := m.session.Compact(6, argStr)
		if err != nil {
			m.chat.AddSystemMessage("⚠ compact: " + err.Error())
			return nil
		}
		m.chat.LoadMessages(m.session.Messages)
		m.chat.AddSystemMessage(fmt.Sprintf("✓ Compacted %d → %d messages\n%s", res.BeforeMsgs, res.AfterMsgs, truncateStr(res.Summary, 200)))
		m.toast = components.NewToast("Compacted", "success", 2*time.Second)

	case "context", "ctx":
		maxCtx := m.maxContextTokens()
		tok := m.totalTokens
		if tok == 0 {
			tok = session.EstimateTokens(m.chat.messages)
		}
		pct := 0.0
		if maxCtx > 0 {
			pct = float64(tok) * 100 / float64(maxCtx)
		}
		users, assts, tools := 0, 0, 0
		for _, msg := range m.chat.messages {
			switch msg.Role {
			case provider.RoleUser:
				users++
			case provider.RoleAssistant:
				assts++
			case provider.RoleTool:
				tools++
			}
		}
		m.chat.AddSystemMessage(fmt.Sprintf(
			`Context window
  Tokens   : %d / %d  (%.1f%%)
  Messages : %d total (%d user · %d assistant · %d tool)
  Cost     : $%.4f
  Session  : %s

  /compact to compress history · auto-compact at ~85%%`,
			tok, maxCtx, pct, len(m.chat.messages), users, assts, tools,
			m.totalCost, func() string {
				if m.session != nil {
					return m.session.ID
				}
				return "?"
			}(),
		))

	case "session-info", "sessioninfo", "si":
		if m.session == nil {
			m.chat.AddSystemMessage("No session")
			return nil
		}
		m.session.Messages = m.chat.messages
		m.session.Tokens = m.totalTokens
		m.session.TotalCost = m.totalCost
		m.chat.AddSystemMessage(m.session.InfoText(m.maxContextTokens()))

	case "rename", "title":
		if m.session == nil || argStr == "" {
			m.chat.AddSystemMessage("Usage: /rename <title>")
			return nil
		}
		m.session.Title = argStr
		m.session.Slug = sessionSlugify(argStr)
		_ = m.session.Save()
		m.chat.AddSystemMessage("Title → " + argStr)

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

	case "theme", "t":
		if theme.MinimalMode() {
			m.chat.AddSystemMessage("Themes unavailable in --minimal mode (terminal-native 16 colors)")
			return nil
		}
		if argStr == "" {
			// Open live-preview picker (Grok parity); bare cycle via /theme cycle
			m.openThemePicker()
			return nil
		}
		if strings.EqualFold(argStr, "cycle") || strings.EqualFold(argStr, "next") {
			name := theme.Cycle()
			m.onThemeApplied()
			m.chat.AddSystemMessage("Theme → " + name)
			m.toast = components.NewToast("Theme: "+name, "info", 2*time.Second)
			return nil
		}
		if theme.SetByName(argStr) {
			m.onThemeApplied()
			m.chat.AddSystemMessage("Theme → " + theme.Name() + "  · color " + theme.ColorLevelName(theme.DetectColorLevel()))
			m.toast = components.NewToast("Theme: "+theme.Name(), "success", 2*time.Second)
			if theme.IsAuto() {
				return autoThemeTick()
			}
		} else {
			m.chat.AddSystemMessage("Unknown theme. Use: " + strings.Join(theme.ThemeNames(), ", ") + "\n  /theme          open picker\n  /theme cycle    next theme")
		}
		return nil

	case "compact-mode":
		on := theme.ToggleCompact()
		m.recalcSizes()
		state := "off"
		if on {
			state = "on"
		}
		m.chat.AddSystemMessage("Compact mode " + state)
		m.toast = components.NewToast("Compact "+state, "info", 2*time.Second)
		return nil

	case "vim-mode", "vim":
		m.vimMode = !m.vimMode
		state := "off"
		if m.vimMode {
			state = "on"
		}
		m.chat.AddSystemMessage("Vim scrollback keys " + state + " (j/k/h/l/g/G/e/E)")
		m.toast = components.NewToast("Vim mode "+state, "info", 2*time.Second)
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
		// Grok: /clear clears chat in-place; /new rotates session id
		m.chat.Clear()
		m.diff = NewDiffModel()
		if m.session != nil {
			m.session.Messages = nil
			m.session.Preview = ""
			_ = m.session.Save()
		}

	case "quit", "q", "exit":
		m.saveSession()
		m.quitting = true
		return tea.Quit

	default:
		// Phase G5: skill slash invoke (/name or /skill:name)
		skillName := cmd
		if strings.HasPrefix(cmd, "skill:") {
			skillName = strings.TrimPrefix(cmd, "skill:")
		}
		if sk, ok := skills.Global().Get(skillName); ok {
			if sk.Disabled {
				m.chat.AddSystemMessage("Skill disabled: " + skillName)
				return nil
			}
			if !sk.UserInvocable {
				m.chat.AddSystemMessage("Skill not user-invocable: " + skillName)
				return nil
			}
			m.toast = components.NewToast("Skill /"+sk.Name, "info", 2*time.Second)
			return m.chat.SubmitAgent(sk.InvokePrompt(argStr))
		}
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
	case ModeAskUser:
		return "ASK"
	case ModePermAsk:
		return "PERM"
	}
	return "?"
}

var slashCommands = []string{
	"/act", "/read", "/ls", "/grep", "/run", "/explain", "/fix",
	"/status", "/commit", "/push", "/pull", "/pr", "/issue", "/gh",
	"/provider", "/model", "/mode", "/cost", "/budget", "/rules", "/index",
	"/theme", "/compact-mode", "/vim-mode",
	"/resume", "/new", "/fork", "/rewind", "/compact", "/context", "/session-info",
	"/mode", "/plan", "/view-plan", "/permissions", "/sandbox", "/hooks",
	"/todos", "/tasks", "/memory", "/skills", "/personas", "/subagents", "/settings", "/copy",
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
	return `CodeForge · Grok 4.5 parity  ·  Phases 1–9 + G1–G7

SCROLLBACK
  Enter · y/Y · j/k · fold · G follow-tail

MODES
  Shift+Tab      BUILD → DESIGN → YOLO

PRODUCT
  /resume /new /fork /rewind /compact /context
  /plan /todos /tasks /subagents /memory /skills /personas /settings
  /theme /permissions /sandbox /hooks /vim-mode /compact-mode

AGENT / IDE
  spawn_subagent background=true · get_subagent_output · resume_from
  /subagents list|show|cancel  ·  /skills  ·  /personas  ·  /sandbox
  See docs/SUBAGENTS.md · docs/SKILLS.md · docs/SANDBOX.md
`
}

func aboutText() string {
	return `CodeForge TUI v1.6.0
Created by NanoMind — 2026 — Apache 2.0

Grok Build TUI–compatible (Phases 1–9 + G1–G8):
  blocks · input · themes · sessions · design plan
  permissions/hooks · todos/tasks · ACP + x.ai/* extensions
  Grok 4.5 · tools · Landlock/Seatbelt · skills · personas
  background subagents · disk-persisted jobs · resume_from
See docs/ACP.md · docs/SUBAGENTS.md · docs/SANDBOX.md
`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
