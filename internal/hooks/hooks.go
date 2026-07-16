// Package hooks runs PreToolUse / PostToolUse scripts (Grok-compatible subset).
package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Event names.
const (
	EventPreToolUse  = "PreToolUse"
	EventPostToolUse = "PostToolUse"
	EventSessionStart = "SessionStart"
)

// HookCommand is one command hook entry.
type HookCommand struct {
	Type    string `json:"type"` // "command"
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"` // seconds
}

// File is the shape of ~/.codeforge/hooks/*.json or project hooks.
type File struct {
	Hooks map[string][]MatcherGroup `json:"hooks"`
}

// MatcherGroup optionally filters by tool name pattern.
type MatcherGroup struct {
	Matcher string        `json:"matcher,omitempty"` // tool name glob, empty = all
	Hooks   []HookCommand `json:"hooks"`
}

// Runner loads and executes hooks.
type Runner struct {
	Groups map[string][]MatcherGroup // event -> groups
	Env    []string
	Cwd    string
}

// Load discovers hooks from global and project dirs.
func Load(workdir string) *Runner {
	r := &Runner{
		Groups: map[string][]MatcherGroup{},
		Cwd:    workdir,
		Env:    os.Environ(),
	}
	// Global
	if home, err := os.UserHomeDir(); err == nil {
		r.loadDir(filepath.Join(home, ".codeforge", "hooks"))
	}
	// Project
	if workdir != "" {
		r.loadDir(filepath.Join(workdir, ".codeforge", "hooks"))
	}
	return r
}

func (r *Runner) loadDir(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var f File
		if json.Unmarshal(data, &f) != nil {
			continue
		}
		for event, groups := range f.Hooks {
			r.Groups[event] = append(r.Groups[event], groups...)
		}
	}
}

// PreToolUse runs blocking hooks. Returns deny reason if any hook denies.
// Exit code 2 (or JSON {"permissionDecision":"deny"}) = deny.
func (r *Runner) PreToolUse(ctx context.Context, toolName, input string) (deny bool, reason string) {
	if r == nil {
		return false, ""
	}
	for _, g := range r.Groups[EventPreToolUse] {
		if g.Matcher != "" && !matchTool(g.Matcher, toolName) {
			continue
		}
		for _, h := range g.Hooks {
			if h.Type != "" && h.Type != "command" {
				continue
			}
			if h.Command == "" {
				continue
			}
			code, out, err := r.runCmd(ctx, h, toolName, input, "")
			if err != nil && code == 0 {
				// execution failure — don't block on infra errors
				continue
			}
			if code == 2 || strings.Contains(out, `"permissionDecision":"deny"`) || strings.Contains(out, "permissionDecision\": \"deny\"") {
				reason = strings.TrimSpace(out)
				if reason == "" {
					reason = "PreToolUse hook denied (exit 2)"
				}
				return true, reason
			}
		}
	}
	return false, ""
}

// PostToolUse runs non-blocking hooks after success.
func (r *Runner) PostToolUse(ctx context.Context, toolName, input, output string, success bool) {
	if r == nil || !success {
		return
	}
	for _, g := range r.Groups[EventPostToolUse] {
		if g.Matcher != "" && !matchTool(g.Matcher, toolName) {
			continue
		}
		for _, h := range g.Hooks {
			if h.Command == "" {
				continue
			}
			_, _, _ = r.runCmd(ctx, h, toolName, input, output)
		}
	}
}

// SessionStart fires SessionStart hooks (non-blocking).
func (r *Runner) SessionStart(ctx context.Context) {
	if r == nil {
		return
	}
	for _, g := range r.Groups[EventSessionStart] {
		for _, h := range g.Hooks {
			if h.Command == "" {
				continue
			}
			_, _, _ = r.runCmd(ctx, h, "", "", "")
		}
	}
}

// Count returns total hook commands loaded.
func (r *Runner) Count() int {
	if r == nil {
		return 0
	}
	n := 0
	for _, groups := range r.Groups {
		for _, g := range groups {
			n += len(g.Hooks)
		}
	}
	return n
}

type payload struct {
	ToolName  string `json:"tool_name"`
	ToolInput string `json:"tool_input"`
	Output    string `json:"tool_output,omitempty"`
	Cwd       string `json:"cwd"`
}

func (r *Runner) runCmd(ctx context.Context, h HookCommand, tool, input, output string) (code int, stdout string, err error) {
	timeout := time.Duration(h.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pl, _ := json.Marshal(payload{ToolName: tool, ToolInput: input, Output: output, Cwd: r.Cwd})
	cmd := exec.CommandContext(cctx, "/bin/sh", "-c", h.Command)
	cmd.Dir = r.Cwd
	cmd.Env = append(r.Env,
		"CODEFORGE_HOOK_TOOL="+tool,
		"CODEFORGE_HOOK_INPUT="+truncate(input, 4000),
		"CODEFORGE_HOOK_CWD="+r.Cwd,
	)
	cmd.Stdin = bytes.NewReader(pl)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	runErr := cmd.Run()
	stdout = buf.String()
	if runErr != nil {
		if ee, ok := runErr.(*exec.ExitError); ok {
			return ee.ExitCode(), stdout, runErr
		}
		return 1, stdout, runErr
	}
	return 0, stdout, nil
}

func matchTool(matcher, name string) bool {
	matcher = strings.TrimSpace(matcher)
	if matcher == "" || matcher == "*" {
		return true
	}
	if strings.HasSuffix(matcher, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(matcher, "*"))
	}
	return strings.EqualFold(matcher, name)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// Summary lists loaded hooks for /hooks UI.
func (r *Runner) Summary() string {
	if r == nil || r.Count() == 0 {
		return "No hooks loaded.\nPlace JSON files in ~/.codeforge/hooks/ or .codeforge/hooks/\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Hooks loaded: %d command(s)\n", r.Count())
	for event, groups := range r.Groups {
		for _, g := range groups {
			m := g.Matcher
			if m == "" {
				m = "*"
			}
			for _, h := range g.Hooks {
				fmt.Fprintf(&b, "  %s [%s] %s\n", event, m, truncate(h.Command, 60))
			}
		}
	}
	return b.String()
}
