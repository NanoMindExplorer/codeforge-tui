// Package research provides a read-only sub-agent tool without import cycles.
package research

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/tool"
)

// Tool is registered into the tool registry from main/cmd.
type Tool struct {
	WorkDir string
	Parent  *tool.Registry
	ProvReg *provider.Registry
}

func (s *Tool) Name() string { return "research" }
func (s *Tool) Description() string {
	return `Spawn a short read-only sub-agent to research the codebase and return a summary.
Use for broad "how does X work?" questions before editing. Does not modify files.`
}

func (s *Tool) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task":           map[string]any{"type": "string"},
			"max_iterations": map[string]any{"type": "integer"},
		},
		"required": []string{"task"},
	}
}

type input struct {
	Task          string `json:"task"`
	MaxIterations int    `json:"max_iterations"`
}

func (s *Tool) Execute(raw json.RawMessage) tool.Result {
	return s.ExecuteStream(raw, nil)
}

func (s *Tool) ExecuteStream(raw []byte, progress tool.ProgressFunc) tool.Result {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return tool.Result{Error: err.Error()}
	}
	if strings.TrimSpace(in.Task) == "" {
		return tool.Result{Error: "task required"}
	}
	if s.ProvReg == nil {
		return tool.Result{Error: "provider not configured for research"}
	}
	p, err := s.ProvReg.Current()
	if err != nil {
		return tool.Result{Error: err.Error()}
	}
	maxIter := in.MaxIterations
	if maxIter <= 0 {
		maxIter = 4
	}
	if maxIter > 6 {
		maxIter = 6
	}

	ro := newReadOnly(s.WorkDir, s.Parent)
	sys := `You are a research sub-agent for CodeForge. READ ONLY.
Use codebase_search, grep_search, read_file, list_dir, diagnostics, fetch_url as needed.
Do not modify files. End with:
## Findings
## Key files
## Risks / open questions`

	msgs := []provider.Message{{Role: provider.RoleUser, Content: in.Task}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if progress != nil {
		progress("research sub-agent started…")
	}

	ch := agent.Run(ctx, agent.Config{
		Provider:      p,
		Tools:         ro,
		System:        sys,
		MaxTokens:     2048,
		MaxIterations: maxIter,
	}, msgs)

	var text strings.Builder
	var toolsUsed []string
	for ev := range ch {
		switch ev.Kind {
		case agent.EventText:
			text.WriteString(ev.Text)
		case agent.EventToolCall:
			toolsUsed = append(toolsUsed, ev.ToolName)
			if progress != nil {
				progress("research: " + ev.ToolName)
			}
		case agent.EventError:
			return tool.Result{Success: false, Error: ev.Error.Error(), Output: text.String()}
		}
	}
	summary := strings.TrimSpace(text.String())
	if summary == "" {
		summary = "(no text produced)"
	}
	return tool.Result{
		Success: true,
		Output:  fmt.Sprintf("RESEARCH SUMMARY (tools: %s)\n\n%s", strings.Join(uniq(toolsUsed), ", "), summary),
	}
}

func newReadOnly(workdir string, parent *tool.Registry) *tool.Registry {
	r := tool.NewReadOnlyRegistry(workdir, parent)
	return r
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
