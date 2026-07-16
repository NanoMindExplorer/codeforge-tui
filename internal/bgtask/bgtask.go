// Package bgtask tracks long-running background shell jobs (Phase 7).
package bgtask

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/codeforge/tui/internal/redact"
)

// Status of a background task.
type Status string

const (
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Cancelled Status = "cancelled"
)

// Task is one background command.
type Task struct {
	ID        string
	Command   string
	WorkDir   string
	Status    Status
	Output    string
	Started   time.Time
	Ended     time.Time
	cancel    context.CancelFunc
}

// Manager holds tasks for a session.
type Manager struct {
	mu    sync.RWMutex
	tasks map[string]*Task
	seq   int
	// OnUpdate optional callback when task finishes (for TUI toast)
	OnUpdate func(t Task)
}

// Global session manager.
var Global = New()

// New creates an empty manager.
func New() *Manager {
	return &Manager{tasks: map[string]*Task{}}
}

// Start launches a shell command in the background.
func (m *Manager) Start(workdir, command string) (*Task, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("empty command")
	}
	m.mu.Lock()
	m.seq++
	id := fmt.Sprintf("bg-%d", m.seq)
	ctx, cancel := context.WithCancel(context.Background())
	t := &Task{
		ID: id, Command: command, WorkDir: workdir,
		Status: Running, Started: time.Now(), cancel: cancel,
	}
	m.tasks[id] = t
	m.mu.Unlock()

	go m.run(ctx, t)
	return t, nil
}

func (m *Manager) run(ctx context.Context, t *Task) {
	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}
	cmd := exec.CommandContext(ctx, shell, flag, t.Command)
	cmd.Dir = t.WorkDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()

	m.mu.Lock()
	t.Ended = time.Now()
	out := redact.Redact(buf.String())
	if len(out) > 12000 {
		out = out[:12000] + "\n… (truncated)"
	}
	t.Output = out
	if ctx.Err() != nil {
		t.Status = Cancelled
	} else if err != nil {
		t.Status = Failed
		if out == "" {
			t.Output = err.Error()
		} else {
			t.Output += "\n" + err.Error()
		}
	} else {
		t.Status = Succeeded
	}
	snap := *t
	cb := m.OnUpdate
	m.mu.Unlock()
	if cb != nil {
		cb(snap)
	}
}

// Cancel stops a running task.
func (m *Manager) Cancel(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return fmt.Errorf("task %s not found", id)
	}
	if t.Status != Running {
		return fmt.Errorf("task %s is %s", id, t.Status)
	}
	if t.cancel != nil {
		t.cancel()
	}
	return nil
}

// Get returns a task copy.
func (m *Manager) Get(id string) (Task, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return Task{}, false
	}
	return *t, true
}

// List returns all tasks newest-first.
func (m *Manager) List() []Task {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, *t)
	}
	// simple reverse by id seq
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// RunningCount returns number of running tasks.
func (m *Manager) RunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, t := range m.tasks {
		if t.Status == Running {
			n++
		}
	}
	return n
}

// Summary for /tasks.
func (m *Manager) Summary() string {
	list := m.List()
	if len(list) == 0 {
		return "No background tasks.\n  run_command with background:true  ·  /tasks cancel <id>"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Background tasks (%d running):\n", m.RunningCount())
	for _, t := range list {
		dur := ""
		if t.Status == Running {
			dur = time.Since(t.Started).Round(time.Second).String()
		} else if !t.Ended.IsZero() {
			dur = t.Ended.Sub(t.Started).Round(time.Second).String()
		}
		fmt.Fprintf(&b, "  %s  [%s]  %s  %s\n", t.ID, t.Status, dur, truncate(t.Command, 50))
	}
	return strings.TrimRight(b.String(), "\n")
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
