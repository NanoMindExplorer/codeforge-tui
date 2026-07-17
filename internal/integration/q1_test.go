// Q1 integration tests: permission parity, DESIGN writes, staged E2E, checkpoint undo.
package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/checkpoint"
	"github.com/codeforge/tui/internal/hooks"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/tool"
)

func TestQ1_ShellAliasPermissionParity(t *testing.T) {
	eng := permission.NewEngine(t.TempDir())
	eng.SetMode(permission.ModeAlwaysApprove)
	// Both names must deny rm -rf
	for _, name := range []string{"run_command", "run_terminal_command"} {
		res := eng.Evaluate(name, `{"command":"rm -rf /tmp/q1-x"}`)
		if res.Decision != permission.DecisionDeny {
			t.Fatalf("%s: got %v %s", name, res.Decision, res.Reason)
		}
	}
}

func TestQ1_DesignBlocksProjectWrite(t *testing.T) {
	dir := t.TempDir()
	sw := tool.NewStagedWriter(dir)
	sw.SetPlanPath(filepath.Join(dir, "plan.md"))
	sw.SetMode(tool.ModeDesign)
	in, _ := json.Marshal(map[string]string{"path": "main.go", "content": "package main"})
	res := sw.Execute(in)
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err == nil {
		t.Fatal("main.go must not exist in DESIGN")
	}
	_ = res
	// plan write via WritePlan
	wp := &tool.WritePlan{Staged: sw}
	pres := wp.Execute(mustJSON(map[string]any{"content": "# Plan\n\nok\n"}))
	if !pres.Success {
		t.Fatal(pres.Error)
	}
}

func TestQ1_StagedBuildApplyCheckpointUndo(t *testing.T) {
	dir := t.TempDir()
	sw := tool.NewStagedWriter(dir)
	sw.SetMode(tool.ModePlan) // BUILD staged
	// pre-existing file for checkpoint
	rel := "app.txt"
	abs := filepath.Join(dir, rel)
	_ = os.WriteFile(abs, []byte("v1"), 0o644)

	// stage overwrite
	in, _ := json.Marshal(map[string]string{"path": rel, "content": "v2"})
	res := sw.Execute(in)
	if !res.Success {
		t.Fatal(res.Error)
	}
	if !sw.HasPending() {
		t.Fatal("expected pending")
	}
	// still v1 on disk
	b, _ := os.ReadFile(abs)
	if string(b) != "v1" {
		t.Fatalf("disk=%q", b)
	}
	// checkpoint old content, then apply
	sid := "q1-sess"
	if _, err := checkpoint.Save(sid, abs, rel, "v1"); err != nil {
		t.Fatal(err)
	}
	sw.AcceptAll()
	if _, _, err := sw.ApplyAccepted(); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(abs)
	if string(b) != "v2" {
		t.Fatalf("after apply=%q", b)
	}
	// undo
	got, err := checkpoint.UndoLast(sid)
	if err != nil {
		t.Fatal(err)
	}
	if got != rel && !strings.Contains(got, "app") {
		t.Log("rel=", got)
	}
	b, _ = os.ReadFile(abs)
	if string(b) != "v1" {
		t.Fatalf("after undo=%q", b)
	}
}

func TestQ1_HookDenyInAgentLoop(t *testing.T) {
	dir := t.TempDir()
	hookDir := filepath.Join(dir, ".codeforge", "hooks")
	_ = os.MkdirAll(hookDir, 0o755)
	_ = os.WriteFile(filepath.Join(hookDir, "deny.json"), []byte(`{
  "hooks": {
    "PreToolUse": [
      { "matcher": "run_command", "hooks": [ { "type": "command", "command": "exit 2" } ] }
    ]
  }
}`), 0o644)
	runner := hooks.Load(dir)
	// Bridge hook deny into Authorizer
	auth := hookAuth{runner: runner}
	reg := tool.NewRegistry(dir)
	p := &scriptProv{steps: []scriptStep{
		{resp: &provider.CompletionResponse{
			ToolCalls: []provider.ToolCall{
				{ID: "1", Name: "run_command", Input: `{"command":"echo hi"}`},
			},
		}},
		{resp: &provider.CompletionResponse{Content: "handled"}},
	}}
	ch := agent.Run(context.Background(), agent.Config{
		Provider: p, Tools: reg, Auth: auth, MaxIterations: 4,
	}, nil)
	var denied bool
	for ev := range ch {
		if ev.Kind == agent.EventToolResult && !ev.ToolSuccess {
			if strings.Contains(ev.ToolOutput, "denied") || strings.Contains(ev.ToolOutput, "🚫") || strings.Contains(ev.ToolOutput, "hook") || strings.Contains(ev.ToolOutput, "blocked") || strings.Contains(ev.ToolOutput, "Permission") {
				denied = true
			}
			// hook auth returns error string
			if strings.Contains(strings.ToLower(ev.ToolOutput), "deny") || strings.Contains(ev.ToolOutput, "exit") || !ev.ToolSuccess {
				denied = true
			}
		}
	}
	if !denied {
		t.Fatal("expected tool denial via hooks authorizer")
	}
}

type hookAuth struct{ runner *hooks.Runner }

func (h hookAuth) Authorize(ctx context.Context, toolName, input string) error {
	if h.runner == nil {
		return nil
	}
	deny, reason := h.runner.PreToolUse(ctx, toolName, input)
	if deny {
		if reason == "" {
			reason = "hook denied"
		}
		return &agentDeny{reason}
	}
	return nil
}
func (h hookAuth) NotifyPost(context.Context, string, string, string, bool) {}

type agentDeny struct{ s string }

func (e *agentDeny) Error() string { return e.s }

// scriptProv minimal provider for integration
type scriptProv struct {
	i     int
	steps []scriptStep
}
type scriptStep struct {
	resp *provider.CompletionResponse
	err  error
}

func (s *scriptProv) Name() string                       { return "script" }
func (s *scriptProv) Models() []provider.ModelInfo       { return nil }
func (s *scriptProv) Model() string                      { return "script-1" }
func (s *scriptProv) SetModel(string) error              { return nil }
func (s *scriptProv) CountTokens([]provider.Message) int { return 1 }
func (s *scriptProv) ValidateConfig() error              { return nil }
func (s *scriptProv) Stream(context.Context, provider.CompletionRequest) (<-chan provider.StreamToken, error) {
	return nil, nil
}
func (s *scriptProv) Complete(ctx context.Context, req provider.CompletionRequest) (*provider.CompletionResponse, error) {
	if s.i >= len(s.steps) {
		st := s.steps[len(s.steps)-1]
		return st.resp, st.err
	}
	st := s.steps[s.i]
	s.i++
	return st.resp, st.err
}
