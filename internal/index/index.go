// Package index builds a lightweight offline codebase index for semantic-ish
// search without embedding APIs (keyword + symbol + path scoring).
package index

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/codeforge/tui/internal/workspace"
)

// Doc is one indexed file.
type Doc struct {
	Path    string // relative display path
	AbsPath string
	Ext     string
	// Symbols extracted (funcs, types, headers)
	Symbols []string
	// Lowercased content sample for scoring (capped)
	Sample string
	Lines  int
}

// Index is an in-memory corpus.
type Index struct {
	mu   sync.RWMutex
	docs []Doc
	root string
}

// Build walks the workspace and indexes text files.
func Build(root string) (*Index, error) {
	idx := &Index{root: root}
	ws := workspace.Get()
	roots := []string{root}
	if ws != nil {
		roots = nil
		for _, r := range ws.ListRoots() {
			roots = append(roots, r.Path)
		}
	}
	for _, r := range roots {
		_ = filepath.WalkDir(r, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				name := d.Name()
				if ws != nil && ws.ShouldSkipDir(name) {
					return filepath.SkipDir
				}
				if name == ".git" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if ws != nil && ws.ShouldSkipFile(name) {
				return nil
			}
			if !isIndexable(name) {
				return nil
			}
			// size cap 256KB
			info, err := d.Info()
			if err != nil || info.Size() > 256*1024 {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil || looksBinary(data) {
				return nil
			}
			rel := path
			if ws != nil {
				rel = ws.RelPath(path)
			} else if r, err := filepath.Rel(root, path); err == nil {
				rel = r
			}
			content := string(data)
			idx.docs = append(idx.docs, Doc{
				Path:    rel,
				AbsPath: path,
				Ext:     strings.ToLower(filepath.Ext(name)),
				Symbols: extractSymbols(name, content),
				Sample:  strings.ToLower(truncate(content, 8000)),
				Lines:   strings.Count(content, "\n") + 1,
			})
			if len(idx.docs) >= 5000 {
				return filepath.SkipAll
			}
			return nil
		})
	}
	return idx, nil
}

// Search ranks documents for a free-text query.
func (idx *Index) Search(query string, limit int) []Hit {
	if idx == nil {
		return nil
	}
	if limit <= 0 {
		limit = 15
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	terms := tokenize(q)
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	type scored struct {
		doc Doc
		sc  float64
	}
	var hits []scored
	for _, d := range idx.docs {
		sc := scoreDoc(d, q, terms)
		if sc > 0 {
			hits = append(hits, scored{d, sc})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].sc > hits[j].sc })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	out := make([]Hit, 0, len(hits))
	for _, h := range hits {
		out = append(out, Hit{
			Path:    h.doc.Path,
			AbsPath: h.doc.AbsPath,
			Score:   h.sc,
			Symbols: h.doc.Symbols,
			Lines:   h.doc.Lines,
			Snippet: snippet(h.doc.Sample, terms),
		})
	}
	return out
}

// Hit is one search result.
type Hit struct {
	Path    string
	AbsPath string
	Score   float64
	Symbols []string
	Lines   int
	Snippet string
}

// Stats returns index size.
func (idx *Index) Stats() (files, symbols int) {
	if idx == nil {
		return 0, 0
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	files = len(idx.docs)
	for _, d := range idx.docs {
		symbols += len(d.Symbols)
	}
	return
}

func scoreDoc(d Doc, q string, terms []string) float64 {
	var sc float64
	pl := strings.ToLower(d.Path)
	if strings.Contains(pl, q) {
		sc += 8
	}
	for _, t := range terms {
		if t == "" {
			continue
		}
		if strings.Contains(pl, t) {
			sc += 3
		}
		for _, sym := range d.Symbols {
			sl := strings.ToLower(sym)
			if sl == t {
				sc += 6
			} else if strings.Contains(sl, t) {
				sc += 3
			}
		}
		if strings.Contains(d.Sample, t) {
			sc += 1
			// density bonus
			sc += float64(strings.Count(d.Sample, t)) * 0.15
		}
	}
	// Prefer source over docs slightly when scores equal-ish
	switch d.Ext {
	case ".go", ".rs", ".ts", ".tsx", ".js", ".py", ".java":
		sc *= 1.1
	case ".md":
		sc *= 0.95
	}
	return sc
}

func extractSymbols(name, content string) []string {
	var syms []string
	ext := strings.ToLower(filepath.Ext(name))
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		switch ext {
		case ".go":
			if strings.HasPrefix(line, "func ") {
				syms = append(syms, takeIdent(strings.TrimPrefix(line, "func ")))
			}
			if strings.HasPrefix(line, "type ") {
				syms = append(syms, takeIdent(strings.TrimPrefix(line, "type ")))
			}
		case ".py":
			if strings.HasPrefix(line, "def ") || strings.HasPrefix(line, "class ") {
				rest := line
				if strings.HasPrefix(rest, "def ") {
					rest = strings.TrimPrefix(rest, "def ")
				} else {
					rest = strings.TrimPrefix(rest, "class ")
				}
				syms = append(syms, takeIdent(rest))
			}
		case ".rs":
			if strings.Contains(line, "fn ") {
				if i := strings.Index(line, "fn "); i >= 0 {
					syms = append(syms, takeIdent(line[i+3:]))
				}
			}
		case ".ts", ".tsx", ".js", ".jsx":
			if strings.HasPrefix(line, "function ") || strings.HasPrefix(line, "export function ") {
				rest := line
				rest = strings.TrimPrefix(rest, "export ")
				rest = strings.TrimPrefix(rest, "function ")
				syms = append(syms, takeIdent(rest))
			}
			if strings.HasPrefix(line, "class ") || strings.HasPrefix(line, "export class ") {
				rest := strings.TrimPrefix(strings.TrimPrefix(line, "export "), "class ")
				syms = append(syms, takeIdent(rest))
			}
		case ".md":
			if strings.HasPrefix(line, "#") {
				h := strings.TrimLeft(line, "#")
				h = strings.TrimSpace(h)
				if h != "" {
					syms = append(syms, h)
				}
			}
		}
		if len(syms) >= 40 {
			break
		}
	}
	return syms
}

func takeIdent(s string) string {
	s = strings.TrimSpace(s)
	// strip receiver (Foo) Bar
	if strings.HasPrefix(s, "(") {
		if i := strings.Index(s, ")"); i >= 0 && i+1 < len(s) {
			s = strings.TrimSpace(s[i+1:])
		}
	}
	var b strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			break
		}
	}
	return b.String()
}

func tokenize(q string) []string {
	fields := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})
	var out []string
	for _, f := range fields {
		f = strings.ToLower(f)
		if len(f) >= 2 {
			out = append(out, f)
		}
	}
	return out
}

func snippet(sample string, terms []string) string {
	if sample == "" {
		return ""
	}
	// find first term occurrence
	pos := 0
	for _, t := range terms {
		if i := strings.Index(sample, t); i >= 0 {
			pos = i
			break
		}
	}
	start := pos - 60
	if start < 0 {
		start = 0
	}
	end := start + 160
	if end > len(sample) {
		end = len(sample)
	}
	s := sample[start:end]
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

func isIndexable(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go", ".py", ".rs", ".js", ".jsx", ".ts", ".tsx", ".java", ".kt", ".c", ".h", ".cpp", ".cc",
		".md", ".txt", ".yaml", ".yml", ".toml", ".json", ".sql", ".sh", ".bash", ".zsh",
		".css", ".scss", ".html", ".vue", ".svelte", ".rb", ".php", ".swift", ".scala":
		return true
	case "":
		return name == "Makefile" || name == "Dockerfile" || name == "go.mod"
	}
	return false
}

func looksBinary(data []byte) bool {
	n := len(data)
	if n > 512 {
		n = 512
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Global index
var (
	gMu  sync.RWMutex
	gIdx *Index
)

func SetGlobal(idx *Index) {
	gMu.Lock()
	gIdx = idx
	gMu.Unlock()
}

func Global() *Index {
	gMu.RLock()
	defer gMu.RUnlock()
	return gIdx
}
