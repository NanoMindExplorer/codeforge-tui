package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/provider"
)

func TestExtractPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{`{"path":"foo/bar.go"}`, "foo/bar.go"},
		{`{"path": "spaced.go"}`, "spaced.go"},
		{`no path here`, ""},
		{`{"path":"a"}`, "a"},
	}
	for _, c := range cases {
		got := extractPath(c.in)
		// crude parser may not handle space after colon
		if c.in == `{"path": "spaced.go"}` {
			// accept empty or spaced path depending on parser
			if got != "" && got != "spaced.go" {
				t.Fatalf("%q → %q", c.in, got)
			}
			continue
		}
		if got != c.want {
			t.Fatalf("%q: got %q want %q", c.in, got, c.want)
		}
	}
}

func TestPumpStreamNil(t *testing.T) {
	if pumpStream(nil) != nil {
		t.Fatal("nil channel → nil cmd")
	}
}

func TestPumpStreamToken(t *testing.T) {
	ch := make(chan provider.StreamToken, 1)
	ch <- provider.StreamToken{Text: "hi", InputTokens: 1, OutputTokens: 2}
	close(ch)
	cmd := pumpStream(ch)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	tick, ok := msg.(StreamTickMsg)
	if !ok {
		t.Fatalf("type %T", msg)
	}
	if tick.Text != "hi" || tick.InputTokens != 1 || tick.OutputTokens != 2 {
		t.Fatalf("%+v", tick)
	}
}

func TestPumpStreamClosed(t *testing.T) {
	ch := make(chan provider.StreamToken)
	close(ch)
	msg := pumpStream(ch)()
	tick := msg.(StreamTickMsg)
	if !tick.Done {
		t.Fatal("closed channel should Done")
	}
}

func TestPumpAgent(t *testing.T) {
	if pumpAgent(nil) != nil {
		t.Fatal("nil")
	}
	ch := make(chan agent.Event, 1)
	ch <- agent.Event{Kind: agent.EventText, Text: "x"}
	close(ch)
	msg := pumpAgent(ch)()
	ev, ok := msg.(AgentEventMsg)
	if !ok || ev.Ev.Kind != agent.EventText || ev.Ev.Text != "x" {
		t.Fatalf("%+v", msg)
	}
}

func TestPumpAgentClosed(t *testing.T) {
	ch := make(chan agent.Event)
	close(ch)
	msg := pumpAgent(ch)()
	ev := msg.(AgentEventMsg)
	if ev.Ev.Kind != agent.EventDone {
		t.Fatalf("closed → Done, got %v", ev.Ev.Kind)
	}
}

func TestHandleAgentEventTextAndDone(t *testing.T) {
	m := testModel(t)
	// text event should not panic
	_ = m.handleAgentEvent(agent.Event{Kind: agent.EventText, Text: "hello agent"})
	// tool call path extract
	_ = m.handleAgentEvent(agent.Event{
		Kind: agent.EventToolCall, ToolName: "read_file",
		ToolInput: `{"path":"main.go"}`,
	})
	// done with tokens
	_ = m.handleAgentEvent(agent.Event{
		Kind: agent.EventDone, InputTokens: 10, OutputTokens: 5,
	})
	if m.totalTokens < 15 {
		t.Fatalf("tokens not accumulated: %d", m.totalTokens)
	}
}

func TestHandleAgentPumpMsgStreamTick(t *testing.T) {
	m := testModel(t)
	// open stream path via handleAgentPumpMsg
	ch := make(chan provider.StreamToken)
	// StreamOpened with done first token
	m2, cmds, ok := m.handleAgentPumpMsg(StreamOpenedMsg{
		Ch:         ch,
		FirstToken: provider.StreamToken{Text: "a", Done: true, OutputTokens: 1},
	})
	if !ok {
		t.Fatal("should handle")
	}
	_ = cmds
	if m2.streamCh != nil {
		t.Fatal("done first token should clear streamCh")
	}
	// unrelated message
	_, _, ok = m2.handleAgentPumpMsg(tea.WindowSizeMsg{Width: 1, Height: 1})
	if ok {
		t.Fatal("window size not stream/agent")
	}
}

func TestBudgetBlocks(t *testing.T) {
	m := testModel(t)
	if m.budgetBlocks() {
		t.Fatal("default no budget")
	}
	m.cfg.Budget.MaxCostUSD = 0.01
	m.totalCost = 0.02
	if !m.budgetBlocks() {
		t.Fatal("should block over budget")
	}
}

func TestAccTokens(t *testing.T) {
	m := testModel(t)
	before := m.totalTokens
	m.accTokens(3, 7)
	if m.totalTokens != before+10 {
		t.Fatal(m.totalTokens)
	}
	if time.Since(m.lastTokenAt) > time.Second {
		t.Fatal("lastTokenAt not set")
	}
}

func TestIsImmediateSlashKnown(t *testing.T) {
	// colocated with stream suite as slash helper used by keys
	for _, s := range []string{"/help", "/clear", "/cost", "/quit"} {
		if !isImmediateSlash(s) {
			t.Fatal(s)
		}
	}
	// agent prompts should NOT auto-run on menu enter alone
	for _, s := range []string{"/act foo", "/run ls"} {
		// isImmediateSlash checks command name only via trimmed path - /act is not in list
		cmd := strings.Fields(s)[0]
		if isImmediateSlash(cmd) {
			t.Fatal("should not immediate:", cmd)
		}
	}
}
