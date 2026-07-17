package acp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/tool"
)

// markerAuth records which session key it represents for bleed detection.
type markerAuth struct {
	id   string
	hits []string
	mu   sync.Mutex
}

func (m *markerAuth) Authorize(ctx context.Context, toolName, input string) error {
	m.mu.Lock()
	m.hits = append(m.hits, toolName)
	m.mu.Unlock()
	return nil
}
func (m *markerAuth) NotifyPost(ctx context.Context, toolName, input, output string, success bool) {
}

// Q6.2 — two registries keep distinct Authorizers; ResolveSubagentAuth does not cross-bleed.
func TestPerSessionAuthorizerIsolation(t *testing.T) {
	a := &markerAuth{id: "sess-A"}
	b := &markerAuth{id: "sess-B"}

	regA := tool.NewRegistry(t.TempDir())
	regB := tool.NewRegistry(t.TempDir())
	regA.Authorizer = a
	regB.Authorizer = b

	// Global points at A — B must still resolve to its own
	tool.SubagentAuthorizer = a
	defer func() { tool.SubagentAuthorizer = nil }()

	gotA := tool.ResolveSubagentAuth(regA)
	gotB := tool.ResolveSubagentAuth(regB)
	if gotA != a {
		t.Fatal("regA should use local auth A")
	}
	if gotB != b {
		t.Fatal("regB should use local auth B, not global A")
	}

	// Concurrent resolve must stay stable
	var wg sync.WaitGroup
	errs := make(chan string, 40)
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if tool.ResolveSubagentAuth(regA) != a {
				errs <- "A bleed"
			}
		}()
		go func() {
			defer wg.Done()
			if tool.ResolveSubagentAuth(regB) != b {
				errs <- "B bleed"
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

// Q6.2 — registry Authorizer works without process-wide global.
func TestRegistryAuthorizerWithoutGlobal(t *testing.T) {
	dir := t.TempDir()
	eng := permission.FromConfig(nil, dir)
	eng.SetMode(permission.ModeAlwaysApprove)
	reg := tool.NewRegistry(dir)
	reg.Authorizer = eng

	old := tool.SubagentAuthorizer
	tool.SubagentAuthorizer = nil
	defer func() { tool.SubagentAuthorizer = old }()
	if tool.ResolveSubagentAuth(reg) != eng {
		t.Fatal("registry authorizer should win without global")
	}
}

// Q6.2 — session/prompt passes the session's auth to the runner (not a swapped global).
func TestPromptUsesSessionAuth(t *testing.T) {
	tx := &bufTransport{}
	var seenAuth agent.Authorizer
	var mu sync.Mutex
	runner := &captureAuthRunner{onRun: func(auth agent.Authorizer) {
		mu.Lock()
		seenAuth = auth
		mu.Unlock()
	}}
	srv := NewServer(Options{
		Version: "test", WorkDir: t.TempDir(), AlwaysApprove: true,
		Quiet: true, Runner: runner,
	})
	srv.SetTransport(tx)

	authA := &markerAuth{id: "A"}
	as := &acpSession{
		ID: "manual-1", WorkDir: t.TempDir(), System: "sys",
		auth: authA, tools: tool.NewRegistry(t.TempDir()),
	}
	as.tools.Authorizer = authA
	srv.mu.Lock()
	srv.sess[as.ID] = as
	srv.mu.Unlock()

	srv.Handle(mustLine(t, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/prompt",
		"params": map[string]any{
			"sessionId": as.ID,
			"prompt":    []map[string]any{{"type": "text", "text": "hi"}},
		},
	}))
	// wait for prompt result
	_ = waitMsgs(t, tx, 1, 3*time.Second)
	mu.Lock()
	got := seenAuth
	mu.Unlock()
	if got != authA {
		t.Fatalf("expected session auth A, got %T %v", got, got)
	}
}

type captureAuthRunner struct {
	onRun func(auth agent.Authorizer)
}

func (c *captureAuthRunner) Run(ctx context.Context, workdir, system string, msgs []provider.Message, auth agent.Authorizer, maxIter int, onEvent func(agent.Event)) {
	if c.onRun != nil {
		c.onRun(auth)
	}
	onEvent(agent.Event{Kind: agent.EventText, Text: "ok"})
	onEvent(agent.Event{Kind: agent.EventDone})
}
