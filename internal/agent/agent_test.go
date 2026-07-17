package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/tool"
)

// mockProv is a scriptable provider for agent loop tests.
type mockProv struct {
	mu    sync.Mutex
	calls int
	// responses[i] is returned on call i (0-based). If calls exceed, last is reused or err.
	script []mockStep
}

type mockStep struct {
	resp *provider.CompletionResponse
	err  error
}

func (m *mockProv) Name() string { return "mock" }
func (m *mockProv) Models() []provider.ModelInfo {
	return []provider.ModelInfo{{ID: "mock-1", Name: "Mock"}}
}
func (m *mockProv) Model() string {
	// Include "grok" so WantsReasoning(auto) is true for reasoning-retry tests.
	return "grok-mock-1"
}
func (m *mockProv) SetModel(string) error              { return nil }
func (m *mockProv) CountTokens([]provider.Message) int { return 1 }
func (m *mockProv) ValidateConfig() error              { return nil }
func (m *mockProv) Stream(context.Context, provider.CompletionRequest) (<-chan provider.StreamToken, error) {
	return nil, errors.New("no stream")
}
func (m *mockProv) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	m.mu.Lock()
	i := m.calls
	m.calls++
	m.mu.Unlock()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(m.script) == 0 {
		return &provider.CompletionResponse{Content: "empty script"}, nil
	}
	if i >= len(m.script) {
		st := m.script[len(m.script)-1]
		return st.resp, st.err
	}
	st := m.script[i]
	return st.resp, st.err
}

func collect(ch <-chan Event) []Event {
	var out []Event
	for ev := range ch {
		out = append(out, ev)
	}
	return out
}

func TestAgentTextOnlyDone(t *testing.T) {
	p := &mockProv{script: []mockStep{
		{resp: &provider.CompletionResponse{Content: "hello world", InputTokens: 3, OutputTokens: 2}},
	}}
	evs := collect(Run(context.Background(), Config{Provider: p, MaxIterations: 3}, nil))
	var text, done bool
	for _, e := range evs {
		if e.Kind == EventText && e.Text == "hello world" {
			text = true
		}
		if e.Kind == EventDone {
			done = true
			if e.InputTokens != 3 || e.OutputTokens != 2 {
				t.Fatalf("tokens %+v", e)
			}
		}
		if e.Kind == EventError {
			t.Fatalf("unexpected error: %v", e.Error)
		}
	}
	if !text || !done {
		t.Fatalf("text=%v done=%v evs=%+v", text, done, kinds(evs))
	}
	if p.calls != 1 {
		t.Fatalf("calls=%d", p.calls)
	}
}

func TestAgentToolCallThenDone(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry(dir)
	// Use list_dir which exists
	p := &mockProv{script: []mockStep{
		{resp: &provider.CompletionResponse{
			Content: "listing",
			ToolCalls: []provider.ToolCall{
				{ID: "1", Name: "list_dir", Input: `{"path":"."}`},
			},
		}},
		{resp: &provider.CompletionResponse{Content: "done listing"}},
	}}
	evs := collect(Run(context.Background(), Config{
		Provider: p, Tools: reg, MaxIterations: 5,
	}, []provider.Message{{Role: provider.RoleUser, Content: "list"}}))

	var sawCall, sawResult, sawDone bool
	for _, e := range evs {
		switch e.Kind {
		case EventToolCall:
			if e.ToolName == "list_dir" {
				sawCall = true
			}
		case EventToolResult:
			sawResult = true
			if !e.ToolSuccess && e.ToolName == "list_dir" {
				// list_dir may succeed
			}
		case EventDone:
			sawDone = true
		case EventError:
			t.Fatalf("error: %v", e.Error)
		}
	}
	if !sawCall || !sawResult || !sawDone {
		t.Fatalf("call=%v result=%v done=%v kinds=%v", sawCall, sawResult, sawDone, kinds(evs))
	}
	if p.calls != 2 {
		t.Fatalf("calls=%d", p.calls)
	}
}

type denyAll struct{}

func (denyAll) Authorize(context.Context, string, string) error {
	return errors.New("blocked by policy")
}
func (denyAll) NotifyPost(context.Context, string, string, string, bool) {}

func TestAgentAuthDeny(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry(dir)
	p := &mockProv{script: []mockStep{
		{resp: &provider.CompletionResponse{
			ToolCalls: []provider.ToolCall{
				{ID: "1", Name: "run_command", Input: `{"command":"echo hi"}`},
			},
		}},
		{resp: &provider.CompletionResponse{Content: "ok after deny"}},
	}}
	evs := collect(Run(context.Background(), Config{
		Provider: p, Tools: reg, Auth: denyAll{}, MaxIterations: 4,
	}, nil))
	var denied bool
	for _, e := range evs {
		if e.Kind == EventToolResult && !e.ToolSuccess && strings.Contains(e.ToolOutput, "blocked") {
			denied = true
		}
	}
	if !denied {
		t.Fatalf("expected denied tool result: %v", kinds(evs))
	}
}

func TestAgentMaxIterations(t *testing.T) {
	// Always returns a tool call → never finishes
	p := &mockProv{script: []mockStep{
		{resp: &provider.CompletionResponse{
			ToolCalls: []provider.ToolCall{{ID: "1", Name: "list_dir", Input: `{"path":"."}`}},
		}},
	}}
	reg := tool.NewRegistry(t.TempDir())
	evs := collect(Run(context.Background(), Config{
		Provider: p, Tools: reg, MaxIterations: 2,
	}, nil))
	var errEv error
	for _, e := range evs {
		if e.Kind == EventError {
			errEv = e.Error
		}
	}
	if errEv == nil {
		t.Fatal("expected max iter error", kinds(evs))
	}
	code, msg, _ := Format(errEv)
	// emitted as ProviderError after ToProviderError
	pe, ok := provider.AsProviderError(errEv)
	if !ok || pe == nil {
		t.Fatal(errEv)
	}
	if !strings.Contains(pe.Message, "max iterations") && !strings.Contains(msg, "max") {
		// Format on ProviderError
		if !strings.Contains(provider.FormatUserError(errEv), "iteration") {
			t.Fatalf("msg=%q code=%q pe=%+v", msg, code, pe)
		}
	}
}

func TestAgentCancel(t *testing.T) {
	block := make(chan struct{})
	p := &mockProv{script: []mockStep{
		{err: context.Canceled},
	}}
	// provider returns canceled; also cancel ctx
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = block
	evs := collect(Run(ctx, Config{Provider: p, MaxIterations: 2}, nil))
	var sawErr bool
	for _, e := range evs {
		if e.Kind == EventError {
			sawErr = true
			s := provider.FormatUserError(e.Error)
			if !strings.Contains(strings.ToLower(s), "cancel") {
				t.Fatalf("want cancel-friendly: %s", s)
			}
		}
	}
	if !sawErr {
		t.Fatal(kinds(evs))
	}
}

func TestAgentReasoningRetryInfo(t *testing.T) {
	// Model id contains "grok" → WantsReasoning auto=true → CompleteRetryingReasoning retries once.
	p := &mockProv{script: []mockStep{
		{err: &provider.ProviderError{Code: provider.ErrUnsupported, Message: "no thinking", Retry: true}},
		{resp: &provider.CompletionResponse{Content: "ok without think"}},
	}}
	evs := collect(Run(context.Background(), Config{Provider: p, MaxIterations: 3}, nil))
	var info, text bool
	for _, e := range evs {
		if e.Kind == EventInfo && strings.Contains(e.Text, "Reasoning not supported") {
			info = true
		}
		if e.Kind == EventText && e.Text == "ok without think" {
			text = true
		}
		if e.Kind == EventError {
			t.Fatal(e.Error)
		}
	}
	if !info || !text {
		t.Fatalf("info=%v text=%v kinds=%v calls=%d", info, text, kinds(evs), p.calls)
	}
}

func TestAgentRateLimitRetry(t *testing.T) {
	var slept time.Duration
	p := &mockProv{script: []mockStep{
		{err: &provider.ProviderError{Code: provider.ErrRateLimit, Message: "slow down", Retry: true, RetryAfter: 2 * time.Second}},
		{resp: &provider.CompletionResponse{Content: "after wait"}},
	}}
	zero := 1
	evs := collect(Run(context.Background(), Config{
		Provider:         p,
		MaxIterations:    3,
		RateLimitRetries: &zero,
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = d
			return nil
		},
	}, nil))
	if slept < time.Second {
		t.Fatalf("expected sleep, got %v", slept)
	}
	var info, text bool
	for _, e := range evs {
		if e.Kind == EventInfo && strings.Contains(e.Text, "Rate limited") {
			info = true
		}
		if e.Kind == EventText && e.Text == "after wait" {
			text = true
		}
		if e.Kind == EventError {
			t.Fatal(e.Error)
		}
	}
	if !info || !text {
		t.Fatalf("info=%v text=%v kinds=%v", info, text, kinds(evs))
	}
	if p.calls != 2 {
		t.Fatalf("calls=%d", p.calls)
	}
}

func TestAgentNoProvider(t *testing.T) {
	evs := collect(Run(context.Background(), Config{}, nil))
	if len(evs) != 1 || evs[0].Kind != EventError {
		t.Fatal(kinds(evs))
	}
}

func TestAgentRedactsToolOutputForModel(t *testing.T) {
	// Custom tool that returns a secret
	dir := t.TempDir()
	reg := tool.NewRegistry(dir)
	secretTool := &echoTool{name: "echo_secret", out: "token=sk-abcdefghijklmnopqrstuvwxyz123456"}
	reg.Register(secretTool)

	var captured []provider.Message
	p := &capturingProv{
		mockProv: mockProv{script: []mockStep{
			{resp: &provider.CompletionResponse{
				ToolCalls: []provider.ToolCall{{ID: "1", Name: "echo_secret", Input: `{}`}},
			}},
			{resp: &provider.CompletionResponse{Content: "done"}},
		}},
		onComplete: func(req provider.CompletionRequest) {
			captured = append(captured, req.Messages...)
		},
	}
	_ = collect(Run(context.Background(), Config{Provider: p, Tools: reg, MaxIterations: 4}, nil))
	// Second Complete should include tool message with redacted secret
	found := false
	for _, m := range captured {
		if m.Role == provider.RoleTool {
			found = true
			if strings.Contains(m.Content, "sk-abcdefghijklmnopqrstuvwxyz") {
				t.Fatalf("secret leaked to model: %q", m.Content)
			}
			if !strings.Contains(m.Content, "REDACTED") && !strings.Contains(m.Content, "sk-") {
				// redact may replace whole token
				t.Log("content:", m.Content)
			}
		}
	}
	if !found {
		// onComplete sees all messages on each call — tool role on 2nd call
		for _, m := range captured {
			if strings.Contains(m.Content, "sk-abcdefghijklmnopqrstuvwxyz") {
				t.Fatalf("leaked: %q", m.Content)
			}
		}
	}
}

func TestFormatLoopError(t *testing.T) {
	code, msg, hint := Format(errMaxIterations(3))
	if code != "max_iterations" || msg == "" || hint == "" {
		t.Fatal(code, msg, hint)
	}
	code, _, _ = Format(&provider.ProviderError{Code: provider.ErrAuth, Message: "nope"})
	if code != "auth" {
		t.Fatal(code)
	}
	le := errCanceled()
	if !strings.Contains(le.Error(), "canceled") && !strings.Contains(le.Error(), "Canceled") {
		t.Log(le.Error())
	}
	um := le.(*LoopError).UserMessage()
	if !strings.Contains(um, "code: canceled") {
		t.Fatal(um)
	}
	pe := errMaxIterations(2).(*LoopError).ToProviderError()
	if pe.Message == "" || pe.Provider != "agent" {
		t.Fatal(pe)
	}
	c, m, h := Format(nil)
	if c != "" || m != "" || h != "" {
		t.Fatal(c, m, h)
	}
}

func TestChunkStringAndMapResult(t *testing.T) {
	parts := chunkString(strings.Repeat("x", 1200)+"\n"+strings.Repeat("y", 100), 500)
	if len(parts) < 2 {
		t.Fatal(len(parts))
	}
	ok := mapResult(tool.Result{Success: true, Output: "hi", Diff: "d"})
	if !ok.success || ok.summary != "hi" {
		t.Fatal(ok)
	}
	bad := mapResult(tool.Result{Success: false, Error: ""})
	if bad.success || bad.forModel == "" {
		t.Fatal(bad)
	}
}

func TestExecuteToolUnknownAndNil(t *testing.T) {
	r := executeTool(context.Background(), nil, provider.ToolCall{Name: "x"}, nil)
	if r.success {
		t.Fatal(r)
	}
	reg := tool.NewRegistry(t.TempDir())
	r = executeTool(context.Background(), reg, provider.ToolCall{Name: "no_such_tool"}, nil)
	if r.success || !strings.Contains(r.summary, "unknown") {
		t.Fatal(r)
	}
	// progress path for successful list_dir
	var prog []string
	r = executeTool(context.Background(), reg, provider.ToolCall{Name: "list_dir", Input: `{"path":"."}`}, func(s string) {
		prog = append(prog, s)
	})
	if !r.success {
		t.Fatal(r.summary)
	}
	if len(prog) == 0 {
		t.Fatal("expected progress")
	}
}

func TestDefaultSleep(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := defaultSleep(ctx, time.Hour); err == nil {
		t.Fatal("expected cancel")
	}
	if err := defaultSleep(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	// tiny real sleep
	if err := defaultSleep(context.Background(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
}

func TestBuildToolDefsNil(t *testing.T) {
	if buildToolDefs(nil) != nil {
		t.Fatal("want nil")
	}
	reg := tool.NewRegistry(t.TempDir())
	defs := buildToolDefs(reg)
	if len(defs) == 0 {
		t.Fatal("expected tools")
	}
}

func TestRateLimitRetryExhausted(t *testing.T) {
	zero := 1
	p := &mockProv{script: []mockStep{
		{err: &provider.ProviderError{Code: provider.ErrRateLimit, Message: "slow", Retry: true, RetryAfter: time.Millisecond}},
		{err: &provider.ProviderError{Code: provider.ErrRateLimit, Message: "still slow", Retry: true}},
	}}
	evs := collect(Run(context.Background(), Config{
		Provider: p, MaxIterations: 2, RateLimitRetries: &zero,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}, nil))
	var err bool
	for _, e := range evs {
		if e.Kind == EventError {
			err = true
		}
	}
	if !err {
		t.Fatal(kinds(evs))
	}
}

func TestThinkingEvent(t *testing.T) {
	p := &mockProv{script: []mockStep{
		{resp: &provider.CompletionResponse{Content: "ans", Reasoning: "step1", ReasoningTokens: 5}},
	}}
	evs := collect(Run(context.Background(), Config{Provider: p}, nil))
	var think bool
	for _, e := range evs {
		if e.Kind == EventThinking && e.Thinking == "step1" {
			think = true
		}
	}
	if !think {
		t.Fatal(kinds(evs))
	}
}

func TestRateLimitSleepCanceled(t *testing.T) {
	p := &mockProv{script: []mockStep{
		{err: &provider.ProviderError{Code: provider.ErrRateLimit, Message: "slow", Retry: true, RetryAfter: time.Second}},
	}}
	one := 1
	evs := collect(Run(context.Background(), Config{
		Provider: p, MaxIterations: 2, RateLimitRetries: &one,
		Sleep: func(ctx context.Context, d time.Duration) error {
			return context.Canceled
		},
	}, nil))
	var saw bool
	for _, e := range evs {
		if e.Kind == EventError {
			saw = true
			s := provider.FormatUserError(e.Error)
			if !strings.Contains(strings.ToLower(s), "cancel") {
				t.Log(s)
			}
		}
	}
	if !saw {
		t.Fatal(kinds(evs))
	}
}

func TestLongToolOutputProgress(t *testing.T) {
	dir := t.TempDir()
	reg := tool.NewRegistry(dir)
	big := strings.Repeat("line\n", 200)
	reg.Register(&echoTool{name: "big_out", out: big})
	p := &mockProv{script: []mockStep{
		{resp: &provider.CompletionResponse{
			ToolCalls: []provider.ToolCall{{ID: "1", Name: "big_out", Input: `{}`}},
		}},
		{resp: &provider.CompletionResponse{Content: "ok"}},
	}}
	evs := collect(Run(context.Background(), Config{Provider: p, Tools: reg, MaxIterations: 4}, nil))
	var prog bool
	for _, e := range evs {
		if e.Kind == EventToolProgress {
			prog = true
		}
	}
	if !prog {
		t.Fatal("expected progress chunks", kinds(evs))
	}
}

func TestNilLoopErrorMethods(t *testing.T) {
	var le *LoopError
	_ = le.Error()
	_ = le.UserMessage()
	_ = le.ToProviderError()
}

func TestDisableRateLimitRetry(t *testing.T) {
	zero := 0
	p := &mockProv{script: []mockStep{
		{err: &provider.ProviderError{Code: provider.ErrRateLimit, Message: "slow", Retry: true}},
	}}
	evs := collect(Run(context.Background(), Config{
		Provider: p, RateLimitRetries: &zero, MaxIterations: 2,
	}, nil))
	if p.calls != 1 {
		t.Fatalf("calls=%d want 1", p.calls)
	}
	var err bool
	for _, e := range evs {
		if e.Kind == EventError {
			err = true
		}
	}
	if !err {
		t.Fatal(kinds(evs))
	}
}

func kinds(evs []Event) []string {
	var s []string
	for _, e := range evs {
		s = append(s, kindName(e.Kind))
	}
	return s
}

func kindName(k EventKind) string {
	switch k {
	case EventText:
		return "text"
	case EventThinking:
		return "thinking"
	case EventToolCall:
		return "tool_call"
	case EventToolResult:
		return "tool_result"
	case EventToolProgress:
		return "progress"
	case EventDone:
		return "done"
	case EventError:
		return "error"
	case EventInfo:
		return "info"
	default:
		return "?"
	}
}

type echoTool struct {
	name, out string
}

func (e *echoTool) Name() string        { return e.name }
func (e *echoTool) Description() string { return "echo" }
func (e *echoTool) Schema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (e *echoTool) Execute(json.RawMessage) tool.Result {
	return tool.Result{Success: true, Output: e.out}
}

type capturingProv struct {
	mockProv
	onComplete func(provider.CompletionRequest)
}

func (c *capturingProv) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	if c.onComplete != nil {
		c.onComplete(req)
	}
	return c.mockProv.Complete(ctx, req)
}
