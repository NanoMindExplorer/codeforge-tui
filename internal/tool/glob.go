package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GlobSearch finds files by glob pattern under the project (Grok-style discovery).
type GlobSearch struct {
	WorkDir string
}

func (g *GlobSearch) Name() string { return "glob_file_search" }
func (g *GlobSearch) Description() string {
	return `Find files by glob pattern under the project root (e.g. **/*.go, src/**/*.ts).
Respects common ignore dirs (node_modules, .git, vendor). Grok-compatible discovery tool.`
}
func (g *GlobSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{
				"type":        "string",
				"description": "Glob pattern relative to project root",
			},
			"path": map[string]any{
				"type":        "string",
				"description": "Optional subdirectory to search under",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Max paths (default 50, max 200)",
			},
		},
		"required": []string{"pattern"},
	}
}

type globInput struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path"`
	MaxResults int    `json:"max_results"`
}

var globIgnore = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".codeforge": true,
	"dist": true, "build": true, "target": true, "__pycache__": true,
	".venv": true, "venv": true, ".idea": true, ".vscode": true,
}

func (g *GlobSearch) Execute(input json.RawMessage) Result {
	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	pat := strings.TrimSpace(in.Pattern)
	if pat == "" {
		return Result{Error: "pattern required"}
	}
	max := in.MaxResults
	if max <= 0 {
		max = 50
	}
	if max > 200 {
		max = 200
	}
	root := g.WorkDir
	if in.Path != "" {
		p, err := resolvePathWS(g.WorkDir, in.Path)
		if err != nil {
			return Result{Error: err.Error()}
		}
		root = p
	}
	// Normalize pattern: if no **, search recursively with **/
	searchPat := pat
	if !strings.Contains(pat, "**") && !strings.HasPrefix(pat, "/") {
		// also try **/pattern
	}

	var matches []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			base := d.Name()
			if globIgnore[base] {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(g.WorkDir, path)
		if err != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)
		// match against pattern in multiple ways
		if matchGlob(searchPat, rel) || matchGlob(filepath.Base(searchPat), d.Name()) {
			matches = append(matches, rel)
		} else if strings.Contains(searchPat, "**") {
			// filepath.Match doesn't support **; simple recursive suffix
			if matchStarStar(searchPat, rel) {
				matches = append(matches, rel)
			}
		}
		if len(matches) >= max {
			return fmt.Errorf("max")
		}
		return nil
	})
	if err != nil && err.Error() != "max" {
		// walk error non-fatal
	}
	sort.Strings(matches)
	if len(matches) == 0 {
		return Result{Success: true, Output: fmt.Sprintf("No files match %q under %s", pat, root)}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Glob %q → %d file(s):\n", pat, len(matches))
	for _, m := range matches {
		b.WriteString("  ")
		b.WriteString(m)
		b.WriteByte('\n')
	}
	return Result{Success: true, Output: b.String()}
}

func matchGlob(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	if err == nil && ok {
		return true
	}
	// also match basename-only patterns against full rel path last segment
	ok, err = filepath.Match(pattern, filepath.Base(name))
	return err == nil && ok
}

// matchStarStar handles simple **/*.ext and **/name patterns.
func matchStarStar(pattern, rel string) bool {
	pattern = filepath.ToSlash(pattern)
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(pattern, "**/") {
		suf := pattern[3:]
		if strings.HasPrefix(suf, "*.") {
			return strings.HasSuffix(rel, suf[1:]) // *.go → ends with .go
		}
		ok, _ := filepath.Match(suf, filepath.Base(rel))
		if ok {
			return true
		}
		// path ends with /suf
		return strings.HasSuffix(rel, "/"+suf) || rel == suf
	}
	if strings.Contains(pattern, "**/") {
		parts := strings.SplitN(pattern, "**/", 2)
		prefix, rest := parts[0], parts[1]
		if prefix != "" && !strings.HasPrefix(rel, strings.TrimSuffix(prefix, "/")) {
			return false
		}
		return matchStarStar("**/"+rest, rel)
	}
	ok, _ := filepath.Match(pattern, rel)
	return ok
}
