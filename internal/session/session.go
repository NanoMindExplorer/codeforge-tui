// Package session persists and resumes CodeForge conversations.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/codeforge/tui/internal/provider"
)

// Session is a persisted conversation.
type Session struct {
	ID        string             `json:"id"`
	Slug      string             `json:"slug"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Provider  string             `json:"provider"`
	Model     string             `json:"model"`
	Workdir   string             `json:"workdir"`
	Messages  []provider.Message `json:"messages"`
	TotalCost float64            `json:"total_cost"`
	Tokens    int                `json:"tokens"`
	Preview   string             `json:"preview"`
}

// Dir returns ~/.codeforge/sessions
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".codeforge", "sessions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// New creates a new in-memory session.
func New(providerName, model, workdir string) *Session {
	now := time.Now()
	id := now.Format("20060102-150405")
	return &Session{
		ID:        id,
		Slug:      "new",
		CreatedAt: now,
		UpdatedAt: now,
		Provider:  providerName,
		Model:     model,
		Workdir:   workdir,
	}
}

// Path returns the JSON path for this session.
func (s *Session) Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	slug := s.Slug
	if slug == "" || slug == "new" {
		slug = "session"
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%s.json", s.ID, slug)), nil
}

// Save writes the session to disk.
func (s *Session) Save() error {
	s.UpdatedAt = time.Now()
	if s.Preview == "" && len(s.Messages) > 0 {
		for _, m := range s.Messages {
			if m.Role == provider.RoleUser {
				s.Preview = truncate(m.Content, 80)
				s.Slug = slugify(m.Content)
				break
			}
		}
	}
	path, err := s.Path()
	if err != nil {
		return err
	}
	// Remove old file if slug changed
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// Load reads a session by ID prefix or full filename stem.
func Load(idOrPath string) (*Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	// Direct path?
	if strings.HasSuffix(idOrPath, ".json") {
		return loadFile(idOrPath)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), idOrPath) {
			return loadFile(filepath.Join(dir, e.Name()))
		}
	}
	return nil, fmt.Errorf("session %q not found", idOrPath)
}

func loadFile(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// List returns sessions newest-first (max n, 0 = all).
func List(max int) ([]Session, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		s, err := loadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash && b.Len() > 0 {
			b.WriteByte('-')
			prevDash = true
		}
		if b.Len() >= 32 {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "session"
	}
	return out
}
