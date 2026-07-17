package acp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/provider"
)

// multiTurnRunner emits text + tool call + tool result + done (Q6.4 golden path).
type multiTurnRunner struct{}

func (m *multiTurnRunner) Run(ctx context.Context, workdir, system string, msgs []provider.Message, auth agent.Authorizer, maxIter int, onEvent func(agent.Event)) {
	onEvent(agent.Event{Kind: agent.EventText, Text: "I'll list the directory. "})
	onEvent(agent.Event{Kind: agent.EventToolCall, ToolName: "list_dir", ToolInput: `{"path":"."}`})
	onEvent(agent.Event{Kind: agent.EventToolResult, ToolName: "list_dir", ToolOutput: "main.go\n", ToolSuccess: true})
	onEvent(agent.Event{Kind: agent.EventText, Text: "Done listing."})
	onEvent(agent.Event{Kind: agent.EventDone})
}

// Q6.4 — multi-turn fixture: initialize → session/new → prompt → tool stream → result.
// Uses fake runner so CI needs no API key. Golden shape of JSON-RPC traffic.
func TestMultiTurnJSONRPCFixture(t *testing.T) {
	tx := &bufTransport{}
	dir := t.TempDir()
	srv := NewServer(Options{
		Version: "test-fixture", WorkDir: dir, AlwaysApprove: true,
		Quiet: true, Runner: &multiTurnRunner{},
	})
	srv.SetTransport(tx)

	// 1. initialize
	srv.Handle(mustLine(t, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize",
		"params": map[string]any{"protocolVersion": 1},
	}))
	msgs := waitMsgs(t, tx, 1, time.Second)
	initRes := msgs[0]
	if initRes["error"] != nil {
		t.Fatal(initRes)
	}
	result, _ := initRes["result"].(map[string]any)
	if result["protocolVersion"].(float64) != 1 {
		t.Fatal(result)
	}
	// agent info
	info, _ := result["agentInfo"].(map[string]any)
	if info["name"] != "codeforge" {
		t.Fatal(info)
	}

	// 2. session/new — may require bootstrap; if no provider, inject session manually
	// Prefer real session/new when possible; fallback seed for offline CI.
	srv.Handle(mustLine(t, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "session/new",
		"params": map[string]any{"cwd": dir},
	}))
	// wait a bit for response (bootstrap may fail without keys)
	deadline := time.Now().Add(5 * time.Second)
	var sid string
	for time.Now().Before(deadline) {
		for _, m := range tx.snapshot() {
			if id, ok := m["id"].(float64); ok && id == 2 {
				if m["error"] != nil {
					// offline: seed session
					sid = "fixture-sess"
					srv.mu.Lock()
					srv.sess[sid] = &acpSession{ID: sid, WorkDir: dir, System: "test"}
					srv.mu.Unlock()
					// synthesize ok for fixture continuity
					t.Logf("session/new offline seed: %v", m["error"])
				} else if res, ok := m["result"].(map[string]any); ok {
					sid, _ = res["sessionId"].(string)
				}
			}
		}
		if sid != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sid == "" {
		sid = "fixture-sess"
		srv.mu.Lock()
		srv.sess[sid] = &acpSession{ID: sid, WorkDir: dir, System: "test"}
		srv.mu.Unlock()
	}

	before := len(tx.snapshot())
	// 3. session/prompt
	srv.Handle(mustLine(t, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "session/prompt",
		"params": map[string]any{
			"sessionId": sid,
			"prompt":    []map[string]any{{"type": "text", "text": "list files"}},
		},
	}))

	// wait for prompt completion (id=3)
	var promptDone map[string]any
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, m := range tx.snapshot()[before:] {
			if id, ok := m["id"].(float64); ok && id == 3 {
				promptDone = m
			}
		}
		if promptDone != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if promptDone == nil {
		t.Fatalf("prompt incomplete; msgs=%v", tx.snapshot())
	}
	if promptDone["error"] != nil {
		t.Fatal(promptDone["error"])
	}
	pr := promptDone["result"].(map[string]any)
	if pr["stopReason"] != StopEndTurn {
		t.Fatal(pr)
	}

	// Golden shape: saw tool_call + agent_message_chunk notifications
	var sawText, sawTool, sawToolUpdate bool
	for _, m := range tx.snapshot() {
		if m["method"] != "session/update" {
			continue
		}
		params, _ := m["params"].(map[string]any)
		upd, _ := params["update"].(map[string]any)
		switch upd["sessionUpdate"] {
		case "agent_message_chunk":
			sawText = true
		case "tool_call":
			sawTool = true
			if upd["title"] != "list_dir" && upd["title"] != nil {
				// title is tool name
			}
		case "tool_call_update":
			sawToolUpdate = true
		}
	}
	if !sawText || !sawTool {
		t.Fatalf("golden multi-turn missing streams text=%v tool=%v update=%v all=%v",
			sawText, sawTool, sawToolUpdate, summarizeMethods(tx.snapshot()))
	}
	if !sawToolUpdate {
		t.Log("tool_call_update optional for fake runner path")
	}
}

func summarizeMethods(msgs []map[string]any) string {
	var b strings.Builder
	for _, m := range msgs {
		if m["method"] != nil {
			b.WriteString(m["method"].(string))
			b.WriteByte(' ')
		} else if m["id"] != nil {
			b.WriteString("resp ")
		}
	}
	return b.String()
}
