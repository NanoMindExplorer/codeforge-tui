package tool

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/redact"
)

// MemorySearch searches cross-session notes stored under ~/.codeforge/memory/.
// Agents (and users via /memory) can append notes; search is simple keyword match.
type MemorySearch struct{}

func (m *MemorySearch) Name() string { return "memory_search" }
func (m *MemorySearch) Description() string {
	return `Search cross-session memory notes (prior decisions, project conventions).
Grok-compatible. Use to recall facts stored with memory_write or /memory add.`
}
func (m *MemorySearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Keywords to search"},
			"limit": map[string]any{"type": "integer", "description": "Max hits (default 8)"},
		},
		"required": []string{"query"},
	}
}

type memorySearchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (m *MemorySearch) Execute(input json.RawMessage) Result {
	var in memorySearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return Result{Error: "query required"}
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 8
	}
	dir, err := memoryDir()
	if err != nil {
		return Result{Error: err.Error()}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Result{Success: true, Output: "(no memory store yet)"}
	}
	terms := strings.Fields(strings.ToLower(q))
	var hits []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		buf := make([]byte, 0, 64*1024)
		sc.Buffer(buf, 1024*1024)
		for sc.Scan() {
			line := sc.Text()
			low := strings.ToLower(line)
			ok := true
			for _, t := range terms {
				if !strings.Contains(low, t) {
					ok = false
					break
				}
			}
			if ok {
				hits = append(hits, line)
				if len(hits) >= limit {
					break
				}
			}
		}
		_ = f.Close()
		if len(hits) >= limit {
			break
		}
	}
	if len(hits) == 0 {
		return Result{Success: true, Output: "No memory hits for: " + q}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Memory hits for %q (%d):\n", q, len(hits))
	for i, h := range hits {
		// parse note field if JSON
		var note struct {
			Text string    `json:"text"`
			At   time.Time `json:"at"`
			Tags string    `json:"tags"`
		}
		if json.Unmarshal([]byte(h), &note) == nil && note.Text != "" {
			fmt.Fprintf(&b, "%d. [%s] %s\n", i+1, note.At.Format("2006-01-02"), note.Text)
		} else {
			fmt.Fprintf(&b, "%d. %s\n", i+1, h)
		}
	}
	return Result{Success: true, Output: redact.Redact(b.String())}
}

// MemoryWrite appends a note to the memory store.
type MemoryWrite struct{}

func (m *MemoryWrite) Name() string { return "memory_write" }
func (m *MemoryWrite) Description() string {
	return `Store a durable note for future sessions (conventions, decisions, credentials NEVER).
Search later with memory_search.`
}
func (m *MemoryWrite) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
			"tags": map[string]any{"type": "string", "description": "optional space-separated tags"},
		},
		"required": []string{"text"},
	}
}

type memoryWriteInput struct {
	Text string `json:"text"`
	Tags string `json:"tags"`
}

func (m *MemoryWrite) Execute(input json.RawMessage) Result {
	var in memoryWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return Result{Error: "text required"}
	}
	if err := AppendMemory(text, in.Tags); err != nil {
		return Result{Error: err.Error()}
	}
	return Result{Success: true, Output: "Memory note stored (" + fmt.Sprintf("%d chars", len(text)) + ")"}
}

// AppendMemory is shared by CLI /memory and the tool.
func AppendMemory(text, tags string) error {
	dir, err := memoryDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "notes.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	rec := map[string]any{
		"at":   time.Now().UTC(),
		"text": text,
		"tags": tags,
	}
	enc := json.NewEncoder(f)
	return enc.Encode(rec)
}

func memoryDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".codeforge", "memory")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// ListMemoryRecent returns the last n note texts (newest first) for /memory list.
func ListMemoryRecent(n int) ([]string, error) {
	if n <= 0 {
		n = 10
	}
	dir, err := memoryDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "notes.jsonl")
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var all []string
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		var note struct {
			Text string    `json:"text"`
			At   time.Time `json:"at"`
			Tags string    `json:"tags"`
		}
		if json.Unmarshal([]byte(line), &note) == nil && note.Text != "" {
			tag := ""
			if note.Tags != "" {
				tag = " [" + note.Tags + "]"
			}
			all = append(all, fmt.Sprintf("%s%s — %s", note.At.Format("2006-01-02"), tag, note.Text))
		} else {
			all = append(all, line)
		}
	}
	// newest last in file → reverse take n
	out := make([]string, 0, n)
	for i := len(all) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, all[i])
	}
	return out, nil
}
