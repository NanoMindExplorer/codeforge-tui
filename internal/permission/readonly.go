package permission

import "strings"

// Read-only tools auto-approve in every mode (unless deny/ask rule or hook).
var readOnlyTools = map[string]bool{
	"read_file":             true,
	"list_dir":              true,
	"list_directory":        true, // Grok alias
	"grep_search":           true,
	"grep":                  true, // Grok alias
	"codebase_search":       true,
	"diagnostics":           true,
	"fetch_url":             true,
	"web_fetch":             true, // Grok alias
	"web_search":            true,
	"write_plan":            true,
	"exit_plan_mode":        true,
	"enter_plan_mode":       true,
	"research":              true,
	"memory_search":         true,
	"todo_write":            true, // Grok treats as soft state
	"ask_user_question":     true,
	"ask_user":              true, // Grok alias
	"spawn_subagent":        true, // gated by subagent mode internally
	"glob_file_search":      true,
	"glob":                  true,
	"find_files":            true,
}

// IsReadOnlyTool reports whether the tool is auto-approved by default.
func IsReadOnlyTool(name string) bool {
	if readOnlyTools[name] {
		return true
	}
	// MCP tools are NOT auto-approved
	return false
}

// Read-only shell primary commands (word-boundary).
var readOnlyShell = map[string]bool{
	"ls": true, "cat": true, "pwd": true, "date": true, "whoami": true,
	"hostname": true, "uptime": true, "ps": true,
	"head": true, "tail": true, "wc": true, "sort": true, "uniq": true,
	"tr": true, "cut": true, "echo": true, "printf": true,
	"grep": true, "rg": true, "find": true, // find can be destructive with -delete; checked separately
	"git": true, // further checked for subcommand
	"go": true,  // go test/build may write; only go list/env/version/fmt -n auto
	"cargo": true,
	"kubectl": true,
	"which": true, "type": true, "file": true, "stat": true, "tree": true,
	"diff": true, "cmp": true,
	"jq": true, "yq": true,
	"env": true, "printenv": true,
}

// git subcommands that are read-only
var gitReadOnly = map[string]bool{
	"status": true, "branch": true, "log": true, "diff": true,
	"show": true, "ls-files": true, "rev-parse": true, "remote": true,
	"describe": true, "tag": true, "blame": true, "stash": true, // stash list only ideally
}

// IsReadOnlyShell reports whether all segments of cmd are read-only.
func IsReadOnlyShell(cmd string) bool {
	for _, seg := range SplitShellSegments(cmd) {
		if !isReadOnlySegment(seg) {
			return false
		}
	}
	return true
}

func isReadOnlySegment(seg string) bool {
	fields := strings.Fields(seg)
	if len(fields) == 0 {
		return true
	}
	base := fields[0]
	// strip path
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)

	switch base {
	case "find":
		// find -delete is not read-only
		for _, f := range fields[1:] {
			if f == "-delete" || f == "-exec" || f == "-ok" {
				return false
			}
		}
		return true
	case "git":
		if len(fields) < 2 {
			return true
		}
		sub := fields[1]
		if sub == "stash" && len(fields) > 2 && fields[2] != "list" && fields[2] != "show" {
			return false
		}
		return gitReadOnly[sub]
	case "go":
		if len(fields) < 2 {
			return true
		}
		switch fields[1] {
		case "list", "env", "version", "doc", "help", "fmt":
			// go fmt writes — only -n is dry
			if fields[1] == "fmt" {
				for _, f := range fields[2:] {
					if f == "-n" {
						return true
					}
				}
				return false
			}
			return true
		default:
			return false
		}
	case "cargo":
		if len(fields) >= 2 && fields[1] == "check" {
			return true
		}
		return false
	case "kubectl":
		if len(fields) >= 2 {
			switch fields[1] {
			case "get", "logs", "describe", "explain", "api-resources", "version":
				return true
			}
		}
		return false
	case "rg":
		for _, f := range fields[1:] {
			if strings.HasPrefix(f, "--pre") {
				return false
			}
		}
		return true
	default:
		return readOnlyShell[base]
	}
}
