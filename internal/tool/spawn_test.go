package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codeforge/tui/internal/personas"
	"github.com/codeforge/tui/internal/provider"
)

func TestResolveSubagentType(t *testing.T) {
	if resolveSubagentType("general-purpose", "") != "general-purpose" {
		t.Fatal()
	}
	if resolveSubagentType("", "explore") != "explore" {
		t.Fatal()
	}
	if resolveSubagentType("plan", "") != "plan" {
		t.Fatal()
	}
}

func TestCapabilityFilter(t *testing.T) {
	dir := t.TempDir()
	base := NewRegistry(dir)
	rw := FilterRegistryByCapability(base, CapReadWrite, dir, nil)
	if _, ok := rw.Get("run_command"); ok {
		t.Fatal("read-write should not have shell")
	}
	if _, ok := rw.Get("write_file"); !ok {
		t.Fatal("read-write should have write")
	}
	ex := FilterRegistryByCapability(base, CapExecute, dir, nil)
	if _, ok := ex.Get("write_file"); ok {
		t.Fatal("execute should not write")
	}
	if _, ok := ex.Get("run_command"); !ok {
		t.Fatal("execute should shell")
	}
	ro := FilterRegistryByCapability(nil, CapReadOnly, dir, nil)
	if _, ok := ro.Get("write_file"); ok {
		t.Fatal("ro no write")
	}
	if _, ok := ro.Get("read_file"); !ok {
		t.Fatal("ro read")
	}
}

func TestPlanRegistry(t *testing.T) {
	r := NewPlanRegistry(t.TempDir(), nil)
	if _, ok := r.Get("write_plan"); !ok {
		t.Fatal("write_plan")
	}
	if _, ok := r.Get("write_file"); ok {
		t.Fatal("no write_file on plan")
	}
}

func TestSpawnPromptAliasAndPersona(t *testing.T) {
	dir := t.TempDir()
	_ = personas.Load(personas.Options{WorkDir: dir})

	SubagentRunner = func(ctx context.Context, workdir, system string, msgs []provider.Message, tools *Registry, maxIter int, onEvent func(SubagentEvent)) {
		if !strings.Contains(system, "system-reminder") {
			onEvent(SubagentEvent{Kind: "error", Error: "missing persona reminder"})
			return
		}
		if len(msgs) == 0 || !strings.Contains(msgs[0].Content, "find TODOs") {
			onEvent(SubagentEvent{Kind: "error", Error: "bad task"})
			return
		}
		onEvent(SubagentEvent{Kind: "text", Text: "found none"})
		onEvent(SubagentEvent{Kind: "done"})
	}
	defer func() { SubagentRunner = nil }()

	s := &SpawnSubagent{WorkDir: dir}
	res := s.Execute(json.RawMessage(`{
		"prompt": "find TODOs",
		"subagent_type": "explore",
		"persona": "researcher",
		"description": "scan todos"
	}`))
	if !res.Success {
		t.Fatal(res.Error)
	}
	if !strings.Contains(res.Output, "found none") {
		t.Fatal(res.Output)
	}
	if !strings.Contains(res.Output, "persona=researcher") {
		t.Fatal(res.Output)
	}
}

func TestSpawnUnknownPersona(t *testing.T) {
	dir := t.TempDir()
	_ = personas.Load(personas.Options{WorkDir: dir})
	SubagentRunner = func(ctx context.Context, workdir, system string, msgs []provider.Message, tools *Registry, maxIter int, onEvent func(SubagentEvent)) {
		onEvent(SubagentEvent{Kind: "text", Text: "x"})
	}
	defer func() { SubagentRunner = nil }()
	s := &SpawnSubagent{WorkDir: dir}
	res := s.Execute(json.RawMessage(`{"task":"hi","persona":"no-such-persona"}`))
	if res.Success || !strings.Contains(res.Error, "unknown persona") {
		t.Fatal(res)
	}
}

func TestCreateWorktreeRequiresGit(t *testing.T) {
	dir := t.TempDir()
	_, err := CreateWorktree(dir, "t")
	if err == nil {
		t.Fatal("expected error on non-git")
	}
}

func TestCreateWorktreeOK(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v %s", err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "t@t.com")
	run("git", "config", "user.name", "t")
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hi"), 0644)
	run("git", "add", ".")
	run("git", "commit", "-m", "init")

	wt, err := CreateWorktree(dir, "unit-test")
	if err != nil {
		t.Fatal(err)
	}
	defer wt.Cleanup()
	if _, err := os.Stat(wt.Path); err != nil {
		t.Fatal(err)
	}
}
