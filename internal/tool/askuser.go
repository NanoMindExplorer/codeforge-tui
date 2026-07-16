package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// PendingUserQuestion is set when the agent calls ask_user_question.
// The TUI can surface it; headless returns the question as tool output.
type PendingUserQuestion struct {
	Question string
	Options  []string
}

var (
	askMu      sync.Mutex
	pendingAsk *PendingUserQuestion
)

// ConsumePendingAsk returns and clears the last question (TUI).
func ConsumePendingAsk() *PendingUserQuestion {
	askMu.Lock()
	defer askMu.Unlock()
	q := pendingAsk
	pendingAsk = nil
	return q
}

// PeekPendingAsk returns without clearing.
func PeekPendingAsk() *PendingUserQuestion {
	askMu.Lock()
	defer askMu.Unlock()
	return pendingAsk
}

// AskUserQuestion is the Grok-compatible clarification tool.
type AskUserQuestion struct{}

func (a *AskUserQuestion) Name() string { return "ask_user_question" }
func (a *AskUserQuestion) Description() string {
	return `Ask the user a clarifying question with optional multiple-choice options.
Use when requirements are ambiguous before making large changes.
The answer will arrive in the next user message — stop and wait.`
}
func (a *AskUserQuestion) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{"type": "string"},
			"options": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
				"description": "Optional choices (2–6)",
			},
		},
		"required": []string{"question"},
	}
}

type askUserInput struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

func (a *AskUserQuestion) Execute(input json.RawMessage) Result {
	var in askUserInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	q := strings.TrimSpace(in.Question)
	if q == "" {
		return Result{Error: "question required"}
	}
	askMu.Lock()
	pendingAsk = &PendingUserQuestion{Question: q, Options: in.Options}
	askMu.Unlock()

	var b strings.Builder
	b.WriteString("QUESTION FOR USER (wait for their reply):\n")
	b.WriteString(q)
	b.WriteByte('\n')
	if len(in.Options) > 0 {
		b.WriteString("\nOptions:\n")
		for i, o := range in.Options {
			fmt.Fprintf(&b, "  %d) %s\n", i+1, o)
		}
	}
	b.WriteString("\nDo not proceed with irreversible changes until the user answers.")
	return Result{Success: true, Output: b.String()}
}
