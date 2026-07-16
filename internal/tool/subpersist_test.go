package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codeforge/tui/internal/provider"
)

func TestSubJobPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEFORGE_SUBAGENTS_DIR", dir)

	m := NewSubJobManager()
	j := &SubJob{
		ID: "sub-42", Description: "test job", AgentType: "explore",
		Status: SubSucceeded, Output: "done-output",
		Started: time.Now().Add(-time.Minute), Ended: time.Now(),
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: "hi"},
			{Role: provider.RoleAssistant, Content: "done-output"},
		},
		System: "sys", MaxIterations: 6,
	}
	m.Put(j)

	// new manager loads
	m2 := NewSubJobManager()
	n, err := m2.LoadFromDisk()
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatal("expected load")
	}
	got, ok := m2.Get("sub-42")
	if !ok {
		t.Fatal("missing job")
	}
	if got.Output != "done-output" || got.AgentType != "explore" {
		t.Fatalf("%+v", got)
	}
	if len(got.Messages) != 2 {
		t.Fatal(got.Messages)
	}
	// seq should be at least 42
	if m2.seq < 42 {
		t.Fatal(m2.seq)
	}
	// file exists
	if _, err := os.Stat(filepath.Join(dir, "sub-42.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunningMarkedFailedOnLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CODEFORGE_SUBAGENTS_DIR", dir)
	m := NewSubJobManager()
	m.Put(&SubJob{
		ID: "sub-7", Status: SubRunning, Started: time.Now(), AgentType: "explore",
	})
	// force status running on disk
	data := []byte(`{"id":"sub-7","status":"running","agent_type":"explore","started":"2020-01-01T00:00:00Z"}`)
	_ = os.WriteFile(filepath.Join(dir, "sub-7.json"), data, 0644)

	m2 := NewSubJobManager()
	_, _ = m2.LoadFromDisk()
	j, ok := m2.Get("sub-7")
	if !ok || j.Status != SubFailed {
		t.Fatalf("%+v", j)
	}
}
