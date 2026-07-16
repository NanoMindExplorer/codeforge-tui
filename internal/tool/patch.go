package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codeforge/tui/internal/diff"
	"github.com/codeforge/tui/internal/sandbox"
	"github.com/codeforge/tui/internal/workspace"
)

// SearchReplace performs precise in-file edits (preferred over full write_file).
// Honors Plan mode via StagedWriter when available on the registry… actually
// SearchReplace is a standalone tool that stages through a shared StagedWriter.
type SearchReplace struct {
	WorkDir string
	Staged  *StagedWriter // optional; if set, Plan mode stages
}

func (s *SearchReplace) Name() string { return "search_replace" }
func (s *SearchReplace) Description() string {
	return `Replace exact text in a file (surgical edit — prefer over write_file for partial changes).
Provide path, old_string (must match uniquely unless replace_all), and new_string.
In Plan mode the result is staged for review like write_file.`
}

func (s *SearchReplace) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "File path relative to project root",
			},
			"old_string": map[string]any{
				"type":        "string",
				"description": "Exact text to find",
			},
			"new_string": map[string]any{
				"type":        "string",
				"description": "Replacement text",
			},
			"replace_all": map[string]any{
				"type":        "boolean",
				"description": "Replace every occurrence (default false = require unique match)",
			},
		},
		"required": []string{"path", "old_string", "new_string"},
	}
}

type searchReplaceInput struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func (s *SearchReplace) Execute(input json.RawMessage) Result {
	var in searchReplaceInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: fmt.Sprintf("invalid: %v", err)}
	}
	if in.Path == "" || in.OldString == "" {
		return Result{Error: "path and old_string required"}
	}
	if in.OldString == in.NewString {
		return Result{Error: "old_string and new_string are identical"}
	}
	path, err := resolvePathWS(s.WorkDir, in.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if err := sandbox.Global().CheckWrite(path); err != nil {
		return Result{Error: err.Error()}
	}
	oldContent, err := os.ReadFile(path)
	if err != nil {
		return Result{Error: fmt.Sprintf("read: %v", err)}
	}
	src := string(oldContent)
	count := strings.Count(src, in.OldString)
	if count == 0 {
		return Result{Error: "old_string not found in file (ensure exact match including whitespace)"}
	}
	if count > 1 && !in.ReplaceAll {
		return Result{Error: fmt.Sprintf("old_string matched %d times — set replace_all=true or make old_string unique", count)}
	}
	var newContent string
	if in.ReplaceAll {
		newContent = strings.ReplaceAll(src, in.OldString, in.NewString)
	} else {
		newContent = strings.Replace(src, in.OldString, in.NewString, 1)
	}
	return s.commit(path, src, newContent, fmt.Sprintf("search_replace ×%d", count))
}

func (s *SearchReplace) commit(absPath, oldContent, newContent, note string) Result {
	rel := relDisplay(s.WorkDir, absPath)
	d := diff.Unified(rel, oldContent, newContent)
	if s.Staged != nil {
		if blocked := s.Staged.DesignBlocked(absPath); blocked != nil {
			return *blocked
		}
		if s.Staged.Mode() == ModePlan {
			s.Staged.Stage(absPath, rel, oldContent, newContent)
			return Result{
				Success: true,
				Output:  fmt.Sprintf("⏳ PENDING review: %s (%s)", rel, note),
				Diff:    d,
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
		return Result{Error: err.Error()}
	}
	if err := os.WriteFile(absPath, []byte(newContent), 0644); err != nil {
		return Result{Error: fmt.Sprintf("write: %v", err)}
	}
	return Result{
		Success: true,
		Output:  fmt.Sprintf("Updated %s (%s)", rel, note),
		Diff:    d,
	}
}

// ApplyPatch applies a multi-hunk unified-style patch or CodeForge patch format.
type ApplyPatch struct {
	WorkDir string
	Staged  *StagedWriter
}

func (a *ApplyPatch) Name() string { return "apply_patch" }
func (a *ApplyPatch) Description() string {
	return `Apply a structured patch to one or more files.
Supports:
1) CodeForge patch format:
   *** Begin Patch
   *** Update File: path/to/file.go
   @@
   -old line
   +new line
   *** Add File: path/new.go
   +content
   *** Delete File: path/old.go
   *** End Patch
2) Or a single-file unified diff with path + patch fields.
Prefer apply_patch / search_replace over rewriting entire files.`
}

func (a *ApplyPatch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"patch": map[string]any{
				"type":        "string",
				"description": "Full patch text (Begin/End Patch format preferred)",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional single-file path when using unified diff body",
			},
		},
		"required": []string{"patch"},
	}
}

type applyPatchInput struct {
	Patch string `json:"patch"`
	Path  string `json:"path"`
}

func (a *ApplyPatch) Execute(input json.RawMessage) Result {
	var in applyPatchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: fmt.Sprintf("invalid: %v", err)}
	}
	if strings.TrimSpace(in.Patch) == "" {
		return Result{Error: "patch required"}
	}
	// Single-file unified via path+patch body
	if in.Path != "" && !strings.Contains(in.Patch, "*** Begin Patch") {
		return a.applyUnifiedSingle(in.Path, in.Patch)
	}
	return a.applyCodeForgePatch(in.Patch)
}

func (a *ApplyPatch) applyCodeForgePatch(patch string) Result {
	ops, err := parseCodeForgePatch(patch)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if len(ops) == 0 {
		return Result{Error: "no file operations found in patch"}
	}
	var outs []string
	var combinedDiff strings.Builder
	for _, op := range ops {
		res := a.applyOp(op)
		if !res.Success {
			return Result{Error: fmt.Sprintf("%s: %s", op.Path, res.Error), Output: strings.Join(outs, "\n")}
		}
		outs = append(outs, res.Output)
		if res.Diff != "" {
			combinedDiff.WriteString(res.Diff)
			combinedDiff.WriteString("\n")
		}
	}
	return Result{Success: true, Output: strings.Join(outs, "\n"), Diff: combinedDiff.String()}
}

type patchOp struct {
	Kind    string // update | add | delete
	Path    string
	Hunks   string // body lines with -/+/space prefixes for update
	Content string // full content for add
}

func parseCodeForgePatch(patch string) ([]patchOp, error) {
	lines := strings.Split(patch, "\n")
	var ops []patchOp
	var cur *patchOp
	flush := func() {
		if cur != nil {
			ops = append(ops, *cur)
			cur = nil
		}
	}
	inPatch := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trim, "*** Begin Patch"):
			inPatch = true
		case strings.HasPrefix(trim, "*** End Patch"):
			flush()
			inPatch = false
		case strings.HasPrefix(trim, "*** Update File:"):
			flush()
			cur = &patchOp{Kind: "update", Path: strings.TrimSpace(strings.TrimPrefix(trim, "*** Update File:"))}
			inPatch = true
		case strings.HasPrefix(trim, "*** Add File:"):
			flush()
			cur = &patchOp{Kind: "add", Path: strings.TrimSpace(strings.TrimPrefix(trim, "*** Add File:"))}
			inPatch = true
		case strings.HasPrefix(trim, "*** Delete File:"):
			flush()
			cur = &patchOp{Kind: "delete", Path: strings.TrimSpace(strings.TrimPrefix(trim, "*** Delete File:"))}
			inPatch = true
		case cur != nil && inPatch:
			if cur.Kind == "add" {
				body := line
				if strings.HasPrefix(body, "+") {
					body = body[1:]
				}
				if cur.Content != "" {
					cur.Content += "\n"
				}
				cur.Content += body
			} else if cur.Kind == "update" {
				if strings.HasPrefix(line, "@@") {
					continue
				}
				if cur.Hunks != "" {
					cur.Hunks += "\n"
				}
				cur.Hunks += line
			}
		}
	}
	flush()
	// Also accept patches without Begin/End markers if they have Update File lines
	if len(ops) == 0 {
		return nil, fmt.Errorf("could not parse patch — use *** Begin Patch / *** Update File: path format")
	}
	return ops, nil
}

func (a *ApplyPatch) applyOp(op patchOp) Result {
	switch op.Kind {
	case "delete":
		path, err := resolvePathWS(a.WorkDir, op.Path)
		if err != nil {
			return Result{Error: err.Error()}
		}
		old := ""
		if b, err := os.ReadFile(path); err == nil {
			old = string(b)
		}
		rel := relDisplay(a.WorkDir, path)
		d := diff.Unified(rel, old, "")
		if a.Staged != nil {
			if blocked := a.Staged.DesignBlocked(path); blocked != nil {
				return *blocked
			}
			if a.Staged.Mode() == ModePlan {
				// stage empty as delete marker — write empty content; review will write ""
				a.Staged.Stage(path, rel, old, "")
				return Result{Success: true, Output: "⏳ PENDING delete: " + rel, Diff: d}
			}
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return Result{Error: err.Error()}
		}
		return Result{Success: true, Output: "Deleted " + rel, Diff: d}
	case "add":
		path, err := resolvePathWS(a.WorkDir, op.Path)
		if err != nil {
			return Result{Error: err.Error()}
		}
		rel := relDisplay(a.WorkDir, path)
		return a.writeViaStage(path, rel, "", op.Content, "add")
	case "update":
		path, err := resolvePathWS(a.WorkDir, op.Path)
		if err != nil {
			return Result{Error: err.Error()}
		}
		oldB, err := os.ReadFile(path)
		if err != nil {
			return Result{Error: fmt.Sprintf("read: %v", err)}
		}
		old := string(oldB)
		neu, err := applyHunkLines(old, op.Hunks)
		if err != nil {
			return Result{Error: err.Error()}
		}
		rel := relDisplay(a.WorkDir, path)
		return a.writeViaStage(path, rel, old, neu, "update")
	default:
		return Result{Error: "unknown op " + op.Kind}
	}
}

func (a *ApplyPatch) writeViaStage(abs, rel, old, neu, note string) Result {
	d := diff.Unified(rel, old, neu)
	if a.Staged != nil {
		if blocked := a.Staged.DesignBlocked(abs); blocked != nil {
			return *blocked
		}
		if a.Staged.Mode() == ModePlan {
			a.Staged.Stage(abs, rel, old, neu)
			return Result{Success: true, Output: fmt.Sprintf("⏳ PENDING review: %s (%s)", rel, note), Diff: d}
		}
	}
	if neu == "" && note == "delete" {
		_ = os.Remove(abs)
		return Result{Success: true, Output: "Deleted " + rel, Diff: d}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		return Result{Error: err.Error()}
	}
	if err := os.WriteFile(abs, []byte(neu), 0644); err != nil {
		return Result{Error: err.Error()}
	}
	return Result{Success: true, Output: fmt.Sprintf("Patched %s (%s)", rel, note), Diff: d}
}

func (a *ApplyPatch) applyUnifiedSingle(path, patchBody string) Result {
	abs, err := resolvePathWS(a.WorkDir, path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	oldB, err := os.ReadFile(abs)
	if err != nil {
		return Result{Error: err.Error()}
	}
	old := string(oldB)
	neu, err := applyHunkLines(old, patchBody)
	if err != nil {
		return Result{Error: err.Error()}
	}
	rel := relDisplay(a.WorkDir, abs)
	return a.writeViaStage(abs, rel, old, neu, "unified")
}

// applyHunkLines applies a sequence of diff lines (-/+/context) as a single
// sequential rewrite: extract old lines from '-', keep context, insert '+'.
// For robustness we use a line-oriented LCS-free approach: if the patch is
// pure search-replace style (blocks of - then +), apply in order.
func applyHunkLines(old, hunks string) (string, error) {
	// Strategy: collect removal and addition streams; if we can find the
	// contiguous "old block" in the file, replace with "new block".
	var oldBlock, newBlock []string
	var contexts []string
	flushBlock := func() (string, string, bool) {
		if len(oldBlock) == 0 && len(newBlock) == 0 {
			return "", "", false
		}
		o := strings.Join(oldBlock, "\n")
		n := strings.Join(newBlock, "\n")
		oldBlock, newBlock = nil, nil
		return o, n, true
	}
	content := old
	for _, line := range strings.Split(hunks, "\n") {
		if line == "" && len(oldBlock) == 0 && len(newBlock) == 0 {
			continue
		}
		if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "diff ") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			if o, n, ok := flushBlock(); ok {
				if o == "" {
					// pure insert not anchored — append
					content = content + "\n" + n
				} else {
					if !strings.Contains(content, o) {
						return "", fmt.Errorf("patch context not found:\n%s", truncatePatch(o, 200))
					}
					content = strings.Replace(content, o, n, 1)
				}
			}
			continue
		}
		if len(line) == 0 {
			continue
		}
		switch line[0] {
		case '-':
			oldBlock = append(oldBlock, line[1:])
			if strings.HasPrefix(line, "- ") {
				oldBlock[len(oldBlock)-1] = line[2:]
			}
		case '+':
			newBlock = append(newBlock, line[1:])
			if strings.HasPrefix(line, "+ ") {
				newBlock[len(newBlock)-1] = line[2:]
			}
		case ' ':
			// context line — include in both for anchoring
			ctx := line[1:]
			if strings.HasPrefix(line, "  ") {
				ctx = line[2:]
			}
			contexts = append(contexts, ctx)
			oldBlock = append(oldBlock, ctx)
			newBlock = append(newBlock, ctx)
		default:
			// treat as context
			oldBlock = append(oldBlock, line)
			newBlock = append(newBlock, line)
		}
	}
	if o, n, ok := flushBlock(); ok {
		if o == "" {
			content = content + "\n" + n
		} else {
			if !strings.Contains(content, o) {
				// try without context-only noise: only pure deletions
				return "", fmt.Errorf("patch context not found:\n%s", truncatePatch(o, 200))
			}
			content = strings.Replace(content, o, n, 1)
		}
	}
	_ = contexts
	return content, nil
}

func truncatePatch(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func resolvePathWS(workdir, path string) (string, error) {
	if workspace.Get() != nil {
		return workspace.Resolve(workdir, path)
	}
	return resolvePath(workdir, path)
}

func relDisplay(workdir, abs string) string {
	if w := workspace.Get(); w != nil {
		return w.RelPath(abs)
	}
	rel, err := filepath.Rel(workdir, abs)
	if err != nil {
		return abs
	}
	return rel
}
