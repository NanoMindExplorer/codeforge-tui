package tool

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codeforge/tui/internal/personas"
	"github.com/codeforge/tui/internal/provider"
)

func TestBackgroundSubagentAndGetOutput(t *testing.T) {
	// Isolate manager. Keep a local pointer so teardown can wait on THIS
	// manager even after the package-level SubJobs var is restored.
	mgr := NewSubJobManager()
	old := SubJobs
	SubJobs = mgr
	oldRunner := SubagentRunner
	t.Cleanup(func() {
		// Drain in-flight background work on the isolated manager before
		// restoring globals (race detector: SubJobs var + SubagentRunner).
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && mgr.RunningCount() > 0 {
			time.Sleep(10 * time.Millisecond)
		}
		// Brief settle so the goroutine can finish notify after status flip.
		time.Sleep(20 * time.Millisecond)
		SubJobs = old
		SubagentRunner = oldRunner
	})

	dir := t.TempDir()
	_ = personas.Load(personas.Options{WorkDir: dir})

	var started sync.WaitGroup
	started.Add(1)
	SubagentRunner = func(ctx context.Context, workdir, system string, msgs []provider.Message, tools *Registry, maxIter int, onEvent func(SubagentEvent)) {
		started.Done()
		// simulate work
		select {
		case <-time.After(80 * time.Millisecond):
			onEvent(SubagentEvent{Kind: "text", Text: "bg-done"})
		case <-ctx.Done():
			onEvent(SubagentEvent{Kind: "error", Error: "cancelled"})
		}
	}

	s := &SpawnSubagent{WorkDir: dir}
	res := s.Execute(json.RawMessage(`{
		"prompt": "scan",
		"background": true,
		"subagent_type": "explore",
		"description": "bg test"
	}`))
	if !res.Success || !strings.Contains(res.Output, "sub-") {
		t.Fatal(res)
	}
	// extract id
	id := ""
	for _, line := range strings.Split(res.Output, "\n") {
		if strings.Contains(line, "Background subagent sub-") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				id = fields[2]
			}
		}
	}
	if id == "" {
		// fallback parse
		if i := strings.Index(res.Output, "sub-"); i >= 0 {
			id = strings.Fields(res.Output[i:])[0]
		}
	}
	if id == "" {
		t.Fatal("no id", res.Output)
	}

	// wait for start then get with wait
	started.Wait()
	g := &GetSubagentOutput{}
	out := g.Execute(json.RawMessage(`{"id":"` + id + `","wait_ms":2000}`))
	if !out.Success {
		t.Fatal(out)
	}
	if !strings.Contains(out.Output, "bg-done") && !strings.Contains(out.Output, "succeeded") {
		// should have finished
		if !strings.Contains(out.Output, "bg-done") {
			t.Fatal(out.Output)
		}
	}
}

func TestResumeFrom(t *testing.T) {
	old := SubJobs
	SubJobs = NewSubJobManager()
	defer func() { SubJobs = old }()

	dir := t.TempDir()
	_ = personas.Load(personas.Options{WorkDir: dir})

	call := 0
	SubagentRunner = func(ctx context.Context, workdir, system string, msgs []provider.Message, tools *Registry, maxIter int, onEvent func(SubagentEvent)) {
		call++
		if call == 1 {
			onEvent(SubagentEvent{Kind: "text", Text: "first-pass"})
			return
		}
		// resume should have prior messages
		if len(msgs) < 2 {
			onEvent(SubagentEvent{Kind: "error", Error: "expected prior context"})
			return
		}
		onEvent(SubagentEvent{Kind: "text", Text: "second-pass"})
	}
	defer func() { SubagentRunner = nil }()

	s := &SpawnSubagent{WorkDir: dir}
	res1 := s.Execute(json.RawMessage(`{"task":"step1","subagent_type":"explore"}`))
	if !res1.Success {
		t.Fatal(res1)
	}
	// id in header "Subagent sub-N"
	id := ""
	for _, f := range strings.Fields(res1.Output) {
		if strings.HasPrefix(f, "sub-") {
			id = f
			break
		}
	}
	if id == "" {
		t.Fatal(res1.Output)
	}

	res2 := s.Execute(json.RawMessage(`{"prompt":"step2","resume_from":"` + id + `"}`))
	if !res2.Success {
		t.Fatal(res2.Error, res2.Output)
	}
	if !strings.Contains(res2.Output, "second-pass") {
		t.Fatal(res2.Output)
	}
	if !strings.Contains(res2.Output, "resumed_from=") {
		t.Fatal(res2.Output)
	}
}

func TestGetSubagentList(t *testing.T) {
	old := SubJobs
	SubJobs = NewSubJobManager()
	defer func() { SubJobs = old }()
	g := &GetSubagentOutput{}
	res := g.Execute(json.RawMessage(`{}`))
	if !res.Success || !strings.Contains(res.Output, "No subagent") {
		t.Fatal(res)
	}
}

func TestGetSubagentRegistered(t *testing.T) {
	r := NewRegistry(t.TempDir())
	for _, n := range []string{"get_subagent_output", "get_command_or_subagent_output", "spawn_subagent"} {
		if _, ok := r.Get(n); !ok {
			t.Fatal("missing", n)
		}
	}
}
