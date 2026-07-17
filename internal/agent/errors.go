package agent

import (
	"fmt"
	"strings"

	"github.com/codeforge/tui/internal/provider"
)

// LoopError is a structured agent-loop failure safe for TUI / headless JSON.
type LoopError struct {
	Code    string // max_iterations | canceled | no_provider | tool | unknown
	Message string
	Hint    string
}

func (e *LoopError) Error() string {
	if e == nil {
		return "agent error"
	}
	if e.Hint != "" {
		return e.Message + " — " + e.Hint
	}
	return e.Message
}

// UserMessage is multi-line TUI-safe text (same shape as ProviderError).
func (e *LoopError) UserMessage() string {
	if e == nil {
		return "⚠ agent error"
	}
	var b strings.Builder
	b.WriteString("⚠ ")
	b.WriteString(e.Message)
	if e.Hint != "" {
		b.WriteString("\n  → ")
		b.WriteString(e.Hint)
	}
	if e.Code != "" {
		b.WriteString("\n  code: ")
		b.WriteString(e.Code)
	}
	return b.String()
}

// ToProviderError adapts LoopError into provider.FormatUserError-friendly shape.
func (e *LoopError) ToProviderError() *provider.ProviderError {
	if e == nil {
		return &provider.ProviderError{Code: provider.ErrUnknown, Message: "agent error", Retry: false}
	}
	code := provider.ErrUnknown
	switch e.Code {
	case "canceled":
		code = provider.ErrTimeout
	case "no_provider":
		code = provider.ErrAuth
	case "max_iterations":
		code = provider.ErrUnknown
	}
	return &provider.ProviderError{
		Code:     code,
		Message:  e.Message,
		Hint:     e.Hint,
		Retry:    e.Code == "max_iterations",
		Provider: "agent",
	}
}

// Format maps any agent/provider error to a stable headless/TUI code + message + hint.
func Format(err error) (code, message, hint string) {
	if err == nil {
		return "", "", ""
	}
	var le *LoopError
	if asLoop(err, &le) {
		return le.Code, le.Message, le.Hint
	}
	pe, ok := provider.AsProviderError(err)
	if ok && pe != nil {
		return string(pe.Code), pe.Message, pe.Hint
	}
	return "unknown", err.Error(), "Retry or run /doctor"
}

func asLoop(err error, dest **LoopError) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*LoopError); ok {
		*dest = e
		return true
	}
	return false
}

func errNoProvider() error {
	return &LoopError{
		Code:    "no_provider",
		Message: "No AI provider configured",
		Hint:    "Run /setup or set an API key, then /provider",
	}
}

func errMaxIterations(n int) error {
	return &LoopError{
		Code:    "max_iterations",
		Message: fmt.Sprintf("Reached max iterations (%d) without a final answer", n),
		Hint:    "Raise --max-iter, narrow the task, or /compact the session",
	}
}

func errCanceled() error {
	return &LoopError{
		Code:    "canceled",
		Message: "Agent turn canceled",
		Hint:    "You stopped the turn, or a new message interrupted it",
	}
}
