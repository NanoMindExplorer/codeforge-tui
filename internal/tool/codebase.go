package tool

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codeforge/tui/internal/index"
)

// CodebaseSearch queries the offline project index.
type CodebaseSearch struct {
	WorkDir string
}

func (c *CodebaseSearch) Name() string { return "codebase_search" }
func (c *CodebaseSearch) Description() string {
	return `Search the project index by keywords/symbols (fast offline, no embeddings).
Use for "where is X implemented?" before grepping. Returns ranked file paths + snippets.`
}

func (c *CodebaseSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{"type": "string", "description": "Natural language or keywords / symbol names"},
			"limit": map[string]any{"type": "integer", "description": "Max results (default 12)"},
		},
		"required": []string{"query"},
	}
}

type codebaseInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

func (c *CodebaseSearch) Execute(input json.RawMessage) Result {
	var in codebaseInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	if strings.TrimSpace(in.Query) == "" {
		return Result{Error: "query required"}
	}
	idx := index.Global()
	if idx == nil {
		// build on demand
		built, err := index.Build(c.WorkDir)
		if err != nil {
			return Result{Error: fmt.Sprintf("index: %v", err)}
		}
		index.SetGlobal(built)
		idx = built
	}
	hits := idx.Search(in.Query, in.Limit)
	if len(hits) == 0 {
		return Result{Success: true, Output: "No matches in project index for: " + in.Query}
	}
	var b strings.Builder
	files, syms := idx.Stats()
	b.WriteString(fmt.Sprintf("Index: %d files, %d symbols — query %q\n\n", files, syms, in.Query))
	for i, h := range hits {
		b.WriteString(fmt.Sprintf("%d. %s  (score %.1f, %d lines)\n", i+1, h.Path, h.Score, h.Lines))
		if len(h.Symbols) > 0 {
			max := 6
			if len(h.Symbols) < max {
				max = len(h.Symbols)
			}
			b.WriteString("   symbols: " + strings.Join(h.Symbols[:max], ", ") + "\n")
		}
		if h.Snippet != "" {
			b.WriteString("   … " + h.Snippet + "\n")
		}
		b.WriteString("\n")
	}
	return Result{Success: true, Output: b.String()}
}
