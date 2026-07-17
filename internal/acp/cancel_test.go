package acp

import (
	"context"
	"testing"
	"time"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/provider"
)

// slowRunner blocks until ctx is cancelled (Q6.5).
type slowRunner struct {
	started chan struct{}
	done    chan struct{}
}

func (s *slowRunner) Run(ctx context.Context, workdir, system string, msgs []provider.Message, auth agent.Authorizer, maxIter int, onEvent func(agent.Event)) {
	close(s.started)
	select {
	case <-ctx.Done():
		onEvent(agent.Event{Kind: agent.EventText, Text: "cancelled"})
		onEvent(agent.Event{Kind: agent.EventDone})
	case <-time.After(10 * time.Second):
		onEvent(agent.Event{Kind: agent.EventText, Text: "timeout-should-not-happen"})
		onEvent(agent.Event{Kind: agent.EventDone})
	}
	close(s.done)
}

func TestSessionCancelInterruptsPrompt(t *testing.T) {
	tx := &bufTransport{}
	runner := &slowRunner{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
	srv := NewServer(Options{
		Version: "test", WorkDir: t.TempDir(), AlwaysApprove: true,
		Quiet: true, Runner: runner,
	})
	srv.SetTransport(tx)

	// seed session
	as := &acpSession{
		ID: "cancel-1", WorkDir: t.TempDir(), System: "sys",
	}
	srv.mu.Lock()
	srv.sess[as.ID] = as
	srv.mu.Unlock()

	// session/prompt runs in a server-side goroutine
	srv.Handle(mustLine(t, map[string]any{
		"jsonrpc": "2.0", "id": 10, "method": "session/prompt",
		"params": map[string]any{
			"sessionId": as.ID,
			"prompt":    []map[string]any{{"type": "text", "text": "long work"}},
		},
	}))

	// wait until runner starts
	select {
	case <-runner.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}

	// cancel notification (no id)
	srv.Handle(mustLine(t, map[string]any{
		"jsonrpc": "2.0", "method": "session/cancel",
		"params": map[string]any{"sessionId": as.ID},
	}))

	select {
	case <-runner.done:
	case <-time.After(3 * time.Second):
		t.Fatal("runner did not finish after cancel")
	}

	// poll for prompt result (id=10) with stopReason cancelled
	found := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range tx.snapshot() {
			if id, ok := m["id"].(float64); ok && id == 10 {
				res, _ := m["result"].(map[string]any)
				if res != nil {
					t.Logf("stopReason=%v", res["stopReason"])
					if res["stopReason"] == StopCancelled {
						found = true
					}
				}
			}
		}
		if found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !found {
		t.Fatalf("no cancelled prompt result; msgs=%v", tx.snapshot())
	}
}

func TestCancelUnknownSessionNoPanic(t *testing.T) {
	srv := NewServer(Options{Version: "test", Quiet: true, Runner: &fakeRunner{}})
	srv.SetTransport(&bufTransport{})
	// should not panic
	srv.Handle(mustLine(t, map[string]any{
		"jsonrpc": "2.0", "method": "session/cancel",
		"params": map[string]any{"sessionId": "does-not-exist"},
	}))
}
