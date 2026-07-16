package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/provider"
)

// SubagentEvent is a simplified event from a nested agent run.
type SubagentEvent struct {
	Kind     string // text | tool_call | error | done
	Text     string
	ToolName string
	Error    string
}

// SubagentRunner is wired from app to avoid tool↔agent import cycles.
// It runs a nested agent and calls onEvent for each event.
var SubagentRunner func(
	ctx context.Context,
	workdir, system string,
	msgs []provider.Message,
	tools *Registry,
	maxIter int,
	onEvent func(SubagentEvent),
)

// SubagentParentRegistry is the parent tool registry (for MCP tools on explore).
var SubagentParentRegistry *Registry

// SpawnSubagent runs a nested agent turn (Grok spawn_subagent parity).
type SpawnSubagent struct {
	WorkDir string
}

func (s *SpawnSubagent) Name() string { return "spawn_subagent" }
func (s *SpawnSubagent) Description() string {
	return `Spawn a parallel sub-agent for a focused subtask and return its final summary.
Modes: explore (read-only, default) | general (may edit when not in DESIGN).
Grok-compatible. Prefer for broad research or multi-file investigation.`
}
func (s *SpawnSubagent) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{"type": "string", "description": "What the sub-agent should do"},
			"mode": map[string]any{
				"type":        "string",
				"description": "explore | general (default explore)",
			},
			"max_iterations": map[string]any{"type": "integer"},
		},
		"required": []string{"task"},
	}
}

type spawnInput struct {
	Task          string `json:"task"`
	Mode          string `json:"mode"`
	MaxIterations int    `json:"max_iterations"`
}

func (s *SpawnSubagent) Execute(input json.RawMessage) Result {
	return s.ExecuteStream(input, nil)
}

func (s *SpawnSubagent) ExecuteStream(input json.RawMessage, progress ProgressFunc) Result {
	var in spawnInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return Result{Error: "task required"}
	}
	if SubagentRunner == nil {
		return Result{Error: "subagent runner not wired"}
	}
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" {
		mode = "explore"
	}
	maxIter := in.MaxIterations
	if maxIter <= 0 {
		maxIter = 6
	}
	if maxIter > 12 {
		maxIter = 12
	}

	var tools *Registry
	if mode == "general" || mode == "general-purpose" {
		tools = NewRegistry(s.WorkDir)
		// prevent recursive spawn
		delete(tools.tools, "spawn_subagent")
		var order []string
		for _, n := range tools.order {
			if n != "spawn_subagent" {
				order = append(order, n)
			}
		}
		tools.order = order
	} else {
		tools = NewReadOnlyRegistry(s.WorkDir, SubagentParentRegistry)
	}

	sys := `You are a CodeForge sub-agent. Complete the task and return a concise summary.
Do not ask the user questions. Prefer read tools before concluding.`
	if mode == "explore" {
		sys += " You are READ-ONLY: do not edit files."
	}

	if progress != nil {
		progress("subagent (" + mode + ") starting…")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	msgs := []provider.Message{{Role: provider.RoleUser, Content: task}}

	var text strings.Builder
	toolsUsed := 0
	var lastErr string
	SubagentRunner(ctx, s.WorkDir, sys, msgs, tools, maxIter, func(ev SubagentEvent) {
		switch ev.Kind {
		case "text":
			text.WriteString(ev.Text)
			if progress != nil && ev.Text != "" {
				progress(truncateRunes(ev.Text, 80))
			}
		case "tool_call":
			toolsUsed++
			if progress != nil {
				progress("tool: " + ev.ToolName)
			}
		case "error":
			lastErr = ev.Error
		}
	})
	if lastErr != "" {
		return Result{Error: "subagent: " + lastErr}
	}
	out := strings.TrimSpace(text.String())
	if out == "" {
		out = "(subagent finished with no text)"
	}
	header := fmt.Sprintf("Subagent [%s] tools=%d\n\n", mode, toolsUsed)
	return Result{Success: true, Output: header + out}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
