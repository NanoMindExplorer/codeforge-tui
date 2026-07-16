package tool

import "strings"

// CapabilityMode filters tools available to a subagent (Grok-compatible).
type CapabilityMode string

const (
	CapReadOnly  CapabilityMode = "read-only"
	CapReadWrite CapabilityMode = "read-write"
	CapExecute   CapabilityMode = "execute"
	CapAll       CapabilityMode = "all"
)

// ParseCapabilityMode normalizes capability_mode input.
func ParseCapabilityMode(s string) CapabilityMode {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "read-only", "readonly", "ro":
		return CapReadOnly
	case "read-write", "readwrite", "rw":
		return CapReadWrite
	case "execute", "exec", "shell":
		return CapExecute
	case "all", "full", "":
		return CapAll
	default:
		return CapAll
	}
}

// writeTools are mutation tools filtered by capability mode.
var writeTools = map[string]bool{
	"write_file": true, "search_replace": true, "edit_file": true,
	"apply_patch": true, "write_plan": true, "exit_plan_mode": true,
	"enter_plan_mode": true,
}

// shellTools run external commands.
var shellTools = map[string]bool{
	"run_command": true, "run_terminal_command": true,
	"diagnostics": true, // may run go test etc.
	"github":      true, // can mutate remote
}

// FilterRegistryByCapability returns a new registry with tools restricted by mode.
// parent is used for MCP inclusion on read-only builds.
func FilterRegistryByCapability(base *Registry, mode CapabilityMode, workdir string, parent *Registry) *Registry {
	if mode == CapReadOnly {
		return NewReadOnlyRegistry(workdir, parent)
	}
	if base == nil {
		base = NewRegistry(workdir)
	}
	if mode == CapAll {
		return cloneWithoutSpawn(base)
	}

	r := &Registry{tools: make(map[string]Tool)}
	for _, t := range base.List() {
		name := t.Name()
		if name == "spawn_subagent" {
			continue // never recurse
		}
		switch mode {
		case CapReadWrite:
			// no shell
			if shellTools[name] {
				continue
			}
			r.Register(t)
		case CapExecute:
			// no writes
			if writeTools[name] {
				continue
			}
			r.Register(t)
		default:
			r.Register(t)
		}
	}
	// ensure core read tools exist for execute mode
	if mode == CapExecute {
		if _, ok := r.Get("read_file"); !ok {
			ro := NewReadOnlyRegistry(workdir, parent)
			for _, t := range ro.List() {
				r.Register(t)
			}
			// re-add shell from base
			if t, ok := base.Get("run_command"); ok {
				r.Register(t)
			}
			if t, ok := base.Get("run_terminal_command"); ok {
				r.Register(t)
			}
		}
	}
	return r
}

func cloneWithoutSpawn(base *Registry) *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	for _, t := range base.List() {
		if t.Name() == "spawn_subagent" {
			continue
		}
		r.Register(t)
	}
	return r
}

// NewPlanRegistry is explore tools + write_plan only (planning subagent).
func NewPlanRegistry(workdir string, parent *Registry) *Registry {
	r := NewReadOnlyRegistry(workdir, parent)
	// plan tools need a staged writer for write_plan
	staged := NewStagedWriter(workdir)
	staged.SetMode(ModeDesign)
	r.Register(&WritePlan{Staged: staged})
	r.Register(&ExitPlanMode{Staged: staged})
	r.Register(&EnterPlanMode{Staged: staged})
	return r
}
