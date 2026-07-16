package tool

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/codeforge/tui/internal/provider"
)

// SubJobStatus for background / tracked subagents (Phase G7).
type SubJobStatus string

const (
	SubRunning   SubJobStatus = "running"
	SubSucceeded SubJobStatus = "succeeded"
	SubFailed    SubJobStatus = "failed"
	SubCancelled SubJobStatus = "cancelled"
)

// SubJob is one tracked subagent run (sync or async).
type SubJob struct {
	ID          string
	Description string
	AgentType   string
	Persona     string
	Isolation   string
	WorkDir     string
	Status      SubJobStatus
	Output      string
	Error       string
	ToolsUsed   int
	Started     time.Time
	Ended       time.Time
	// Messages for resume_from (user task + assistant summary)
	Messages []provider.Message
	// Config snapshot for resume
	System        string
	MaxIterations int
	// SessionID links to CodeForge chat session when known.
	SessionID string
	// cancel running job
	cancel context.CancelFunc
}

// SubJobManager tracks subagent jobs for the session.
type SubJobManager struct {
	mu    sync.RWMutex
	jobs  map[string]*SubJob
	seq   int
	// OnUpdate optional TUI toast hook
	OnUpdate func(j SubJob)
}

// SubJobs is the process-wide manager.
var SubJobs = NewSubJobManager()

// NewSubJobManager creates an empty manager.
func NewSubJobManager() *SubJobManager {
	return &SubJobManager{jobs: map[string]*SubJob{}}
}

// AllocID reserves a new job id.
func (m *SubJobManager) AllocID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	return fmt.Sprintf("sub-%d", m.seq)
}

// Put registers a job (running or finished) and persists to disk.
func (m *SubJobManager) Put(j *SubJob) {
	m.mu.Lock()
	m.jobs[j.ID] = j
	m.mu.Unlock()
	_ = j.Save()
}

// Get returns a job copy by id.
func (m *SubJobManager) Get(id string) (SubJob, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	j, ok := m.jobs[strings.TrimSpace(id)]
	if !ok || j == nil {
		return SubJob{}, false
	}
	return *j, true
}

// update mutates a job under lock and persists afterward.
func (m *SubJobManager) update(id string, fn func(*SubJob)) bool {
	m.mu.Lock()
	j, ok := m.jobs[id]
	if !ok || j == nil {
		m.mu.Unlock()
		return false
	}
	fn(j)
	snap := *j
	m.mu.Unlock()
	_ = snap.Save()
	return true
}

// List returns jobs newest-first.
func (m *SubJobManager) List() []SubJob {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]SubJob, 0, len(m.jobs))
	for _, j := range m.jobs {
		out = append(out, *j)
	}
	// simple newest first
	for i := 0; i < len(out); i++ {
		for k := i + 1; k < len(out); k++ {
			if out[k].Started.After(out[i].Started) {
				out[i], out[k] = out[k], out[i]
			}
		}
	}
	return out
}

// RunningCount returns in-flight jobs.
func (m *SubJobManager) RunningCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	n := 0
	for _, j := range m.jobs {
		if j.Status == SubRunning {
			n++
		}
	}
	return n
}

// Cancel stops a running job.
func (m *SubJobManager) Cancel(id string) error {
	m.mu.Lock()
	j, ok := m.jobs[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown subagent id %s", id)
	}
	if j.Status != SubRunning {
		m.mu.Unlock()
		return fmt.Errorf("subagent %s is not running (%s)", id, j.Status)
	}
	cancel := j.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Summary text for /subagents.
func (m *SubJobManager) Summary() string {
	jobs := m.List()
	if len(jobs) == 0 {
		return "No subagent jobs yet.\n\nspawn_subagent background=true → returns id\nget_subagent_output id=<id> · /subagents"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Subagent jobs (%d, %d running):\n", len(jobs), m.RunningCount())
	for _, j := range jobs {
		icon := "·"
		switch j.Status {
		case SubRunning:
			icon = "⟳"
		case SubSucceeded:
			icon = "✓"
		case SubFailed:
			icon = "✗"
		case SubCancelled:
			icon = "⏹"
		}
		desc := j.Description
		if desc == "" {
			desc = j.AgentType
		}
		fmt.Fprintf(&b, "  %s %s  %s  [%s]", icon, j.ID, desc, j.Status)
		if j.Persona != "" {
			fmt.Fprintf(&b, " persona=%s", j.Persona)
		}
		b.WriteByte('\n')
		if j.Status != SubRunning && j.Output != "" {
			snip := j.Output
			if len(snip) > 100 {
				snip = snip[:97] + "…"
			}
			snip = strings.ReplaceAll(snip, "\n", " ")
			fmt.Fprintf(&b, "      %s\n", snip)
		}
	}
	b.WriteString("\n/subagents show <id> · /subagents cancel <id>")
	return b.String()
}

// FormatJobOutput builds the tool-visible result for a job.
func FormatJobOutput(j SubJob) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Subagent %s status=%s type=%s", j.ID, j.Status, j.AgentType)
	if j.Persona != "" {
		fmt.Fprintf(&b, " persona=%s", j.Persona)
	}
	if j.Isolation != "" && j.Isolation != "none" {
		fmt.Fprintf(&b, " isolation=%s", j.Isolation)
	}
	fmt.Fprintf(&b, " tools=%d\n", j.ToolsUsed)
	if j.Description != "" {
		fmt.Fprintf(&b, "label: %s\n", j.Description)
	}
	if j.Error != "" {
		fmt.Fprintf(&b, "error: %s\n", j.Error)
	}
	b.WriteByte('\n')
	if j.Output != "" {
		b.WriteString(j.Output)
	} else if j.Status == SubRunning {
		b.WriteString("(still running — poll get_subagent_output again)")
	} else {
		b.WriteString("(no output)")
	}
	return b.String()
}

// notify fires OnUpdate if set.
func (m *SubJobManager) notify(j *SubJob) {
	if m.OnUpdate == nil || j == nil {
		return
	}
	m.OnUpdate(*j)
}
