package sandbox

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// LogEvent appends a sandbox event to ~/.codeforge/sandbox-events.jsonl.
func LogEvent(kind string, fields map[string]any) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	path := filepath.Join(home, ".codeforge", "sandbox-events.jsonl")
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	rec := map[string]any{
		"at":   time.Now().UTC(),
		"kind": kind,
	}
	for k, v := range fields {
		rec[k] = v
	}
	_ = json.NewEncoder(f).Encode(rec)
}
