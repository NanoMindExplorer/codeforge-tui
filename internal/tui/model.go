package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/git"
	gh "github.com/codeforge/tui/internal/github"
	"github.com/codeforge/tui/internal/hooks"
	"github.com/codeforge/tui/internal/keymap"
	"github.com/codeforge/tui/internal/onboarding"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/tui/sessionpicker"
	"github.com/codeforge/tui/internal/tui/slashmenu"
	"github.com/codeforge/tui/internal/tui/themepicker"
	"github.com/codeforge/tui/internal/ui/askuser"
	"github.com/codeforge/tui/internal/ui/blockview"
	"github.com/codeforge/tui/internal/ui/components"
	"github.com/codeforge/tui/internal/ui/filepicker"
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

	chat     ChatModel
	diff     DiffModel
	context  ContextModel
	status   StatusBarModel
	command  CommandModel
	palette  palette.Model
	picker   filepicker.Model
	review   review.Model
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

	// Q2.4: injectable process collaborators (defaults to package globals)
	svc AppServices

	// Q5.3: last user turn for retry after provider error
	lastUserPrompt string
	retryAvailable bool
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
	// Nested spawn_subagent inherits the same permission engine (registry-local first).
	tool.SubagentAuthorizer = permEng
	if toolReg != nil {
		toolReg.Authorizer = permEng
	}
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
		svc:         DefaultAppServices(),
	}
	// Wire plan.md path + BUILD write mode (staged)
	m.syncWriteMode()
	// Align permission mode with session YOLO if always_approve
	if permEng != nil && permEng.GetMode() == permission.ModeAlwaysApprove {
		m.sessionMode = tool.SessionYolo
		m.syncWriteMode()
	}
	// Q5.1: single status card (no rules/hooks flood on first paint)
	healthy := onboarding.ProviderHealthy(provReg)
	activeName := provReg.CurrentName()
	activeModel := ""
	if cur, err := provReg.Current(); err == nil {
		activeModel = cur.Model()
	}
	if onboarding.ShouldShowWelcome(healthy) {
		m.chat.AddSystemMessage(onboarding.StatusCard(cfg, activeName, activeModel, healthy))
		if healthy {
			_ = onboarding.MarkWelcomeShown()
		}
	}
	// Q5.2 empty-state hints (one line each, only when relevant)
	if !healthy {
		m.chat.AddSystemMessage(onboarding.EmptyStateNoKey())
	} else if onboarding.ProjectLooksEmpty(workdir) {
		m.chat.AddSystemMessage(onboarding.EmptyStateNoProject(workdir))
	}
	// Rules/hooks: compact one-liner only (avoid multi-block flood)
	var extras []string
	if rb != nil && len(rb.Paths) > 0 {
		extras = append(extras, fmt.Sprintf("%d rule file(s)", len(rb.Paths)))
	}
	if hookRunner != nil && hookRunner.Count() > 0 {
		extras = append(extras, fmt.Sprintf("%d hook(s)", hookRunner.Count()))
	}
	if len(extras) > 0 {
		m.chat.AddSystemMessage("Project: " + strings.Join(extras, " · ") + "  ·  /rules · /hooks")
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
		return m.handleKeyMsg(msg)

	case StreamOpenedMsg, StreamTickMsg, AgentOpenedMsg, AgentEventMsg:
		var handled bool
		var more []tea.Cmd
		m, more, handled = m.handleAgentPumpMsg(msg)
		if handled {
			cmds = append(cmds, more...)
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
		m.chat.AddSystemMessage(provider.FormatUserError(msg.err))
		m.chat.streaming = false
		// Q5.3: offer retry when we still have a last user prompt
		if strings.TrimSpace(m.lastUserPrompt) != "" {
			m.retryAvailable = true
			m.chat.AddSystemMessage("↻ Retry: Ctrl+R or /retry")
			m.toast = components.NewToast("Error · Ctrl+R to retry", "error", 4*time.Second)
		} else {
			m.toast = components.NewToast(provider.FormatUserErrorShort(msg.err), "error", 4*time.Second)
		}
	}

	m.syncStatus()
	return m, tea.Batch(cmds...)
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
	m.status.TodoBadge = m.todosList().Badge()
	m.status.BgTasks = m.bgTasks().RunningCount()
	m.status.Sandbox = m.sandboxEng().Label()
	m.status.NeedSetup = !onboarding.ProviderHealthy(m.providerReg)
	m.status.KeyCount = onboarding.CountPresentKeys()
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
