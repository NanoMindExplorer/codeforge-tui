package tool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/provider"
)

// persistedSubJob is the on-disk shape (no cancel func).
type persistedSubJob struct {
	ID            string             `json:"id"`
	Description   string             `json:"description"`
	AgentType     string             `json:"agent_type"`
	Persona       string             `json:"persona,omitempty"`
	Isolation     string             `json:"isolation,omitempty"`
	WorkDir       string             `json:"workdir,omitempty"`
	Status        SubJobStatus       `json:"status"`
	Output        string             `json:"output,omitempty"`
	Error         string             `json:"error,omitempty"`
	ToolsUsed     int                `json:"tools_used,omitempty"`
	Started       time.Time          `json:"started"`
	Ended         time.Time          `json:"ended,omitempty"`
	Messages      []provider.Message `json:"messages,omitempty"`
	System        string             `json:"system,omitempty"`
	MaxIterations int                `json:"max_iterations,omitempty"`
	SessionID     string             `json:"session_id,omitempty"`
}

// subJobsDir returns ~/.codeforge/subagents/
func subJobsDir() (string, error) {
	if d := os.Getenv("CODEFORGE_SUBAGENTS_DIR"); d != "" {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", err
		}
		return d, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".codeforge", "subagents")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// PersistPath for a single job.
func (j *SubJob) persistPath() (string, error) {
	dir, err := subJobsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, j.ID+".json"), nil
}

// Save persists a finished (or running snapshot) job to disk.
func (j *SubJob) Save() error {
	if j == nil || j.ID == "" {
		return nil
	}
	// Don't persist huge outputs forever — cap
	out := j.Output
	if len(out) > 200_000 {
		out = out[:200_000] + "\n… (truncated for disk)"
	}
	msgs := j.Messages
	// cap messages
	if len(msgs) > 40 {
		msgs = msgs[len(msgs)-40:]
	}
	rec := persistedSubJob{
		ID: j.ID, Description: j.Description, AgentType: j.AgentType,
		Persona: j.Persona, Isolation: j.Isolation, WorkDir: j.WorkDir,
		Status: j.Status, Output: out, Error: j.Error, ToolsUsed: j.ToolsUsed,
		Started: j.Started, Ended: j.Ended, Messages: msgs,
		System: j.System, MaxIterations: j.MaxIterations, SessionID: j.SessionID,
	}
	path, err := j.persistPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SessionID optional link to CodeForge session.
func (m *SubJobManager) setSessionID(id, sessionID string) {
	m.update(id, func(j *SubJob) { j.SessionID = sessionID })
}

// LoadFromDisk reloads non-running jobs from ~/.codeforge/subagents/.
// Running jobs on disk are marked failed (process died).
func (m *SubJobManager) LoadFromDisk() (int, error) {
	dir, err := subJobsDir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	n := 0
	maxSeq := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var rec persistedSubJob
		if json.Unmarshal(data, &rec) != nil || rec.ID == "" {
			continue
		}
		// bump seq from id sub-N
		if strings.HasPrefix(rec.ID, "sub-") {
			var num int
			if ok, _ := parseSubNum(rec.ID, &num); ok && num > maxSeq {
				maxSeq = num
			}
		}
		st := rec.Status
		if st == SubRunning {
			st = SubFailed
			if rec.Error == "" {
				rec.Error = "process exited while running"
			}
		}
		j := &SubJob{
			ID: rec.ID, Description: rec.Description, AgentType: rec.AgentType,
			Persona: rec.Persona, Isolation: rec.Isolation, WorkDir: rec.WorkDir,
			Status: st, Output: rec.Output, Error: rec.Error, ToolsUsed: rec.ToolsUsed,
			Started: rec.Started, Ended: rec.Ended, Messages: rec.Messages,
			System: rec.System, MaxIterations: rec.MaxIterations, SessionID: rec.SessionID,
		}
		m.mu.Lock()
		if _, exists := m.jobs[j.ID]; !exists {
			m.jobs[j.ID] = j
			n++
		}
		m.mu.Unlock()
	}
	m.mu.Lock()
	if maxSeq > m.seq {
		m.seq = maxSeq
	}
	m.mu.Unlock()
	return n, nil
}

func parseSubNum(id string, n *int) (bool, error) {
	if !strings.HasPrefix(id, "sub-") {
		return false, os.ErrInvalid
	}
	x := 0
	for _, c := range id[4:] {
		if c < '0' || c > '9' {
			break
		}
		x = x*10 + int(c-'0')
	}
	*n = x
	return true, nil
}

// PersistAll saves every job (used on shutdown / after updates).
func (m *SubJobManager) PersistAll() {
	for _, j := range m.List() {
		jj := j
		_ = jj.Save()
	}
}

// autoPersist after update
func (m *SubJobManager) persistID(id string) {
	j, ok := m.Get(id)
	if !ok {
		return
	}
	_ = j.Save()
}
