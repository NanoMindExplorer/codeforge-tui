// StagedWriter implements Plan-mode gated writes: write_file calls are
// captured as pending patches instead of writing to disk immediately.
package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/codeforge/tui/internal/diff"
	"github.com/codeforge/tui/internal/sandbox"
)

// WriteMode controls whether writes stage (Plan/BUILD), apply immediately
// (Act/YOLO), or are restricted to the design plan file (Design).
type WriteMode int

const (
	ModePlan WriteMode = iota // BUILD: stage writes for review
	ModeAct                   // YOLO: apply writes immediately
	ModeDesign                // DESIGN: only plan.md may be written
)

// PendingPatch is a staged write awaiting user approval.
type PendingPatch struct {
	Path       string // absolute
	RelPath    string
	OldContent string
	NewContent string
	Diff       string
	Accepted   bool // for multi-file review UI
}

// StagedWriter wraps FileWriter with Plan/Act/Design gating.
type StagedWriter struct {
	inner    *FileWriter
	mu       sync.Mutex
	mode     WriteMode
	pending  []PendingPatch
	planPath string // absolute path to session plan.md (Design mode allowlist)
}

// NewStagedWriter creates a BUILD-mode writer (staged, default).
func NewStagedWriter(workDir string) *StagedWriter {
	return &StagedWriter{
		inner: &FileWriter{WorkDir: workDir},
		mode:  ModePlan,
	}
}

func (s *StagedWriter) Name() string { return "write_file" }
func (s *StagedWriter) Description() string {
	return "Create or overwrite a file in the project with new content"
}
func (s *StagedWriter) Schema() map[string]any { return s.inner.Schema() }

// SetMode switches Plan/Act/Design write gating.
func (s *StagedWriter) SetMode(m WriteMode) {
	s.mu.Lock()
	s.mode = m
	s.mu.Unlock()
}

// Mode returns current write mode.
func (s *StagedWriter) Mode() WriteMode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.mode
}

// SetPlanPath sets the absolute path of the design plan file (plan.md).
func (s *StagedWriter) SetPlanPath(abs string) {
	s.mu.Lock()
	s.planPath = abs
	s.mu.Unlock()
}

// PlanPath returns the configured plan file path.
func (s *StagedWriter) PlanPath() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.planPath
}

// IsPlanFile reports whether absPath is the allowed design plan file.
func (s *StagedWriter) IsPlanFile(absPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.planPath == "" {
		return false
	}
	return filepath.Clean(absPath) == filepath.Clean(s.planPath)
}

// DesignBlocked returns an error result when Design mode forbids this path.
func (s *StagedWriter) DesignBlocked(absPath string) *Result {
	s.mu.Lock()
	mode := s.mode
	plan := s.planPath
	s.mu.Unlock()
	if mode != ModeDesign {
		return nil
	}
	if plan != "" && filepath.Clean(absPath) == filepath.Clean(plan) {
		return nil
	}
	msg := "DESIGN mode: only the plan file may be edited"
	if plan != "" {
		msg = fmt.Sprintf("DESIGN mode: only %s may be edited (use write_plan or write_file on that path)", plan)
	}
	return &Result{Error: msg}
}

// Pending returns a copy of staged patches.
func (s *StagedWriter) Pending() []PendingPatch {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PendingPatch, len(s.pending))
	copy(out, s.pending)
	return out
}

// HasPending reports if any patches await review.
func (s *StagedWriter) HasPending() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending) > 0
}

// ClearPending discards all staged patches.
func (s *StagedWriter) ClearPending() {
	s.mu.Lock()
	s.pending = nil
	s.mu.Unlock()
}

// SetAccepted marks a pending patch by index as accepted/rejected.
func (s *StagedWriter) SetAccepted(idx int, accepted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idx >= 0 && idx < len(s.pending) {
		s.pending[idx].Accepted = accepted
	}
}

// AcceptAll marks every pending patch accepted.
func (s *StagedWriter) AcceptAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.pending {
		s.pending[i].Accepted = true
	}
}

// RejectAll marks every pending patch rejected.
func (s *StagedWriter) RejectAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.pending {
		s.pending[i].Accepted = false
	}
}

// Stage records a pending write without going through Execute JSON.
// Used by search_replace / apply_patch in Plan mode.
func (s *StagedWriter) Stage(absPath, relPath, oldContent, newContent string) {
	d := diff.Unified(relPath, oldContent, newContent)
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, p := range s.pending {
		if p.Path == absPath {
			s.pending[i] = PendingPatch{
				Path: absPath, RelPath: relPath,
				OldContent: oldContent, NewContent: newContent,
				Diff: d, Accepted: true,
			}
			return
		}
	}
	s.pending = append(s.pending, PendingPatch{
		Path: absPath, RelPath: relPath,
		OldContent: oldContent, NewContent: newContent,
		Diff: d, Accepted: true,
	})
}

// AppliedFile holds info about a file just written (for checkpoint).
type AppliedFile struct {
	AbsPath    string
	RelPath    string
	OldContent string
}

// ApplyAccepted writes accepted patches to disk and clears pending.
// Returns applied files (for checkpoint) and a combined diff of applied ones.
func (s *StagedWriter) ApplyAccepted() (applied []AppliedFile, combinedDiff string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var sb string
	var remaining []PendingPatch
	for _, p := range s.pending {
		if !p.Accepted {
			remaining = append(remaining, p)
			continue
		}
		// Empty new content with non-empty old = delete file
		if p.NewContent == "" && p.OldContent != "" {
			if err := os.Remove(p.Path); err != nil && !os.IsNotExist(err) {
				return applied, combinedDiff, fmt.Errorf("delete %s: %w", p.Path, err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(p.Path), 0755); err != nil {
				return applied, combinedDiff, fmt.Errorf("mkdir %s: %w", p.Path, err)
			}
			if err := os.WriteFile(p.Path, []byte(p.NewContent), 0644); err != nil {
				return applied, combinedDiff, fmt.Errorf("write %s: %w", p.Path, err)
			}
		}
		applied = append(applied, AppliedFile{
			AbsPath:    p.Path,
			RelPath:    p.RelPath,
			OldContent: p.OldContent,
		})
		sb += p.Diff + "\n"
	}
	s.pending = remaining
	return applied, sb, nil
}

func (s *StagedWriter) Execute(input json.RawMessage) Result {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: fmt.Sprintf("invalid: %v", err)}
	}
	if in.Path == "" {
		return Result{Error: "path required"}
	}
	path, err := resolvePathWS(s.inner.WorkDir, in.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if err := sandbox.Global().CheckWrite(path); err != nil {
		return Result{Error: err.Error()}
	}

	oldContent := ""
	if data, err := os.ReadFile(path); err == nil {
		oldContent = string(data)
	}
	rel := relDisplay(s.inner.WorkDir, path)
	d := diff.Unified(rel, oldContent, in.Content)

	s.mu.Lock()
	mode := s.mode
	s.mu.Unlock()

	if mode == ModeDesign {
		if blocked := s.DesignBlocked(path); blocked != nil {
			return *blocked
		}
		// Plan file: write immediately (auto-approved in design mode)
		return s.inner.Execute(input)
	}

	if mode == ModeAct {
		// Immediate write (YOLO / always-approve)
		return s.inner.Execute(input)
	}

	// BUILD (ModePlan): stage only
	s.mu.Lock()
	// Replace existing pending for same path
	found := false
	for i, p := range s.pending {
		if p.Path == path {
			s.pending[i] = PendingPatch{
				Path: path, RelPath: rel,
				OldContent: oldContent, NewContent: in.Content,
				Diff: d, Accepted: true, // default accept in review
			}
			found = true
			break
		}
	}
	if !found {
		s.pending = append(s.pending, PendingPatch{
			Path: path, RelPath: rel,
			OldContent: oldContent, NewContent: in.Content,
			Diff: d, Accepted: true,
		})
	}
	s.mu.Unlock()

	return Result{
		Success: true,
		Output:  fmt.Sprintf("⏳ PENDING review: %s (%d bytes staged)", rel, len(in.Content)),
		Diff:    d,
	}
}

// Ensure NewRegistry uses StagedWriter
// (patched in tool.go below via Registry helpers)

// GetStagedWriter extracts StagedWriter from registry if present.
func (r *Registry) GetStagedWriter() *StagedWriter {
	t, ok := r.Get("write_file")
	if !ok {
		return nil
	}
	if sw, ok := t.(*StagedWriter); ok {
		return sw
	}
	return nil
}
