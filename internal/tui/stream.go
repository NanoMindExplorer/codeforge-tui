package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/checkpoint"
	gh "github.com/codeforge/tui/internal/github"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/ui/components"
)

// handleAgentPumpMsg processes stream/agent pump messages (Q2.3).
// Returns handled=false for unrelated message types.
func (m Model) handleAgentPumpMsg(msg tea.Msg) (Model, []tea.Cmd, bool) {
	var cmds []tea.Cmd
	switch msg := msg.(type) {
	case StreamOpenedMsg:
		m.streamCh = msg.Ch
		nc, c := m.chat.Update(StreamTickMsg{
			Text: msg.FirstToken.Text, Reasoning: msg.FirstToken.Reasoning, Done: msg.FirstToken.Done,
			InputTokens: msg.FirstToken.InputTokens, OutputTokens: msg.FirstToken.OutputTokens,
			ReasoningTokens: msg.FirstToken.ReasoningTokens,
			Error:           msg.FirstToken.Error,
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

	default:
		return m, nil, false
	}
	return m, cmds, true
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
			Text: tok.Text, Reasoning: tok.Reasoning, Done: tok.Done,
			InputTokens: tok.InputTokens, OutputTokens: tok.OutputTokens,
			ReasoningTokens: tok.ReasoningTokens,
			Error:           tok.Error,
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
	ms := theme.AnimationFrameMS()
	if ms < 16 {
		ms = 16
	}
	return tea.Tick(time.Duration(ms)*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg{}
	})
}

// Pump / agent / UI message types used by Update and handleAgentPumpMsg.
type errMsg struct{ err error }

type StreamOpenedMsg struct {
	Ch         <-chan provider.StreamToken
	FirstToken provider.StreamToken
}

type StreamTickMsg struct {
	Text            string
	Reasoning       string
	Done            bool
	InputTokens     int
	OutputTokens    int
	ReasoningTokens int
	Error           error
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
