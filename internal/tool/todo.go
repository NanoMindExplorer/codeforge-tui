package tool

import (
	"encoding/json"
	"fmt"

	"github.com/codeforge/tui/internal/todos"
)

// TodoWrite lets the agent manage the session task list.
type TodoWrite struct{}

func (t *TodoWrite) Name() string { return "todo_write" }
func (t *TodoWrite) Description() string {
	return `Create or update the session TODO list shown in the footer (e.g. 2/5).
Use for multi-step work. merge=true updates by id; merge=false replaces the list.
Statuses: pending | in_progress | completed | cancelled`
}
func (t *TodoWrite) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"todos": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":      map[string]any{"type": "string"},
						"content": map[string]any{"type": "string"},
						"status":  map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "cancelled"}},
					},
					"required": []string{"content"},
				},
			},
			"merge": map[string]any{
				"type":        "boolean",
				"description": "If true, merge by id; if false, replace list (default true)",
			},
		},
		"required": []string{"todos"},
	}
}

type todoWriteInput struct {
	Todos []todos.Item `json:"todos"`
	Merge *bool        `json:"merge"`
}

func (t *TodoWrite) Execute(input json.RawMessage) Result {
	var in todoWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: fmt.Sprintf("invalid: %v", err)}
	}
	if len(in.Todos) == 0 {
		return Result{Error: "todos array required"}
	}
	merge := true
	if in.Merge != nil {
		merge = *in.Merge
	}
	if merge {
		todos.Global.Merge(in.Todos)
	} else {
		todos.Global.Set(in.Todos)
	}
	d, tot := todos.Global.Counts()
	return Result{
		Success: true,
		Output:  fmt.Sprintf("Todos updated: %d/%d complete\n%s", d, tot, todos.Global.Render()),
	}
}
