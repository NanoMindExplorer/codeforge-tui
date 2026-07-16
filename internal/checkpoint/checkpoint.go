// Package checkpoint stores pre-write file snapshots for /undo.
package checkpoint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry is one saved file version.
type Entry struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Path      string    `json:"path"` // absolute path of the file
	RelPath   string    `json:"rel_path"`
	SavedAt   time.Time `json:"saved_at"`
	// content stored as file on disk
}

// Dir returns ~/.codeforge/checkpoints/<sessionID>
func Dir(sessionID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".codeforge", "checkpoints", sessionID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// Save stores the current content of path before it is overwritten.
// oldContent is the bytes that were on disk (may be empty for new files).
func Save(sessionID, absPath, relPath, oldContent string) (string, error) {
	dir, err := Dir(sessionID)
	if err != nil {
		return "", err
	}
	id := time.Now().Format("20060102-150405.000")
	safe := strings.ReplaceAll(relPath, string(filepath.Separator), "__")
	metaName := fmt.Sprintf("%s_%s.meta", id, safe)
	dataName := fmt.Sprintf("%s_%s.data", id, safe)

	meta := fmt.Sprintf("%s\n%s\n%s\n", absPath, relPath, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(dir, metaName), []byte(meta), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, dataName), []byte(oldContent), 0644); err != nil {
		return "", err
	}
	return id, nil
}

// UndoLast restores the most recent checkpoint for sessionID and returns
// the relative path restored.
func UndoLast(sessionID string) (relPath string, err error) {
	dir, err := Dir(sessionID)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var metas []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".meta") {
			metas = append(metas, e.Name())
		}
	}
	if len(metas) == 0 {
		return "", fmt.Errorf("no checkpoints to undo")
	}
	sort.Strings(metas)
	last := metas[len(metas)-1]
	metaPath := filepath.Join(dir, last)
	dataPath := strings.TrimSuffix(metaPath, ".meta") + ".data"

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return "", err
	}
	lines := strings.SplitN(string(metaBytes), "\n", 3)
	if len(lines) < 2 {
		return "", fmt.Errorf("corrupt checkpoint meta")
	}
	absPath := lines[0]
	relPath = lines[1]

	data, err := os.ReadFile(dataPath)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		// File was new — remove it
		_ = os.Remove(absPath)
	} else {
		if err := os.MkdirAll(filepath.Dir(absPath), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(absPath, data, 0644); err != nil {
			return "", err
		}
	}
	// Remove used checkpoint
	_ = os.Remove(metaPath)
	_ = os.Remove(dataPath)
	return relPath, nil
}
