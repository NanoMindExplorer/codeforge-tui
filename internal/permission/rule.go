package permission

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Effect is the rule action.
type Effect string

const (
	EffectDeny  Effect = "deny"
	EffectAsk   Effect = "ask"
	EffectAllow Effect = "allow"
)

// Rule matches a tool invocation.
type Rule struct {
	Tool    string `json:"tool" yaml:"tool" mapstructure:"tool"`       // name or class: run_command, write_file, bash, read, edit, *
	Pattern string `json:"pattern" yaml:"pattern" mapstructure:"pattern"` // glob against command or path
	Effect  Effect `json:"effect" yaml:"effect" mapstructure:"effect"` // deny | ask | allow
}

// Match reports whether this rule applies to tool+input.
func (r Rule) Match(toolName, input string) bool {
	if r.Tool == "" && r.Pattern == "" {
		return false
	}
	if !toolMatches(r.Tool, toolName) {
		return false
	}
	if r.Pattern == "" || r.Pattern == "*" {
		return true
	}
	subject := matchSubject(toolName, input)
	return patternMatch(r.Pattern, subject)
}

func toolMatches(ruleTool, name string) bool {
	if ruleTool == "" || ruleTool == "*" {
		return true
	}
	rt := strings.ToLower(ruleTool)
	n := strings.ToLower(name)
	if rt == n {
		return true
	}
	// class aliases
	switch rt {
	case "bash", "shell", "command":
		return n == "run_command"
	case "read":
		return n == "read_file" || n == "list_dir"
	case "edit", "write":
		return n == "write_file" || n == "search_replace" || n == "apply_patch"
	case "grep", "search":
		return n == "grep_search" || n == "codebase_search"
	case "plan":
		return n == "write_plan" || n == "exit_plan_mode" || n == "enter_plan_mode"
	case "mcp":
		return strings.HasPrefix(n, "mcp_")
	}
	// prefix class: mcp_*
	if strings.HasSuffix(rt, "*") {
		return strings.HasPrefix(n, strings.TrimSuffix(rt, "*"))
	}
	return false
}

// matchSubject extracts command or path from JSON tool input for pattern matching.
func matchSubject(toolName, input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return toolName
	}
	var m map[string]any
	if json.Unmarshal([]byte(input), &m) == nil {
		for _, k := range []string{"command", "path", "query", "url", "content"} {
			if v, ok := m[k]; ok {
				if s, ok := v.(string); ok && s != "" {
					if k == "content" && len(s) > 200 {
						return s[:200]
					}
					return s
				}
			}
		}
	}
	return input
}

// patternMatch supports * wildcards (filepath.Match + prefix* form).
func patternMatch(pattern, subject string) bool {
	pattern = strings.TrimSpace(pattern)
	subject = strings.TrimSpace(subject)
	if pattern == "" || pattern == "*" {
		return true
	}
	// "cmd *" prefix style
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		if strings.HasPrefix(subject, prefix) {
			return true
		}
	}
	// filepath-style glob
	ok, err := filepath.Match(pattern, subject)
	if err == nil && ok {
		return true
	}
	// also try matching first token / path base
	if i := strings.IndexAny(subject, " \t"); i > 0 {
		ok, _ = filepath.Match(pattern, subject[:i])
		if ok {
			return true
		}
	}
	// substring fallback for simple contains patterns without *
	if !strings.Contains(pattern, "*") && !strings.Contains(pattern, "?") {
		return subject == pattern || strings.HasPrefix(subject, pattern+" ")
	}
	return false
}
