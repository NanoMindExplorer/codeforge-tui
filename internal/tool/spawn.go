package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/personas"
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

// SubagentAuth gates nested agent tools (set from TUI/headless/ACP permission engine).
// Same shape as agent.Authorizer without importing agent (cycle avoidance).
type SubagentAuth interface {
	Authorize(ctx context.Context, toolName, input string) error
	NotifyPost(ctx context.Context, toolName, input, output string, success bool)
}

// SubagentAuthorizer is the active gate for spawn_subagent children (may be nil).
var SubagentAuthorizer SubagentAuth

// SpawnSubagent runs a nested agent turn (Grok spawn_subagent parity — Phase G6/G7).
type SpawnSubagent struct {
	WorkDir string
}

func (s *SpawnSubagent) Name() string { return "spawn_subagent" }
func (s *SpawnSubagent) Description() string {
	return `Spawn a focused sub-agent (Grok-compatible).

subagent_type: explore | plan | general-purpose
Optional: capability_mode, isolation (none|worktree), persona,
description, max_iterations, background (return id immediately),
resume_from (continue a finished subagent by id).

When background=true, poll with get_subagent_output (or get_command_or_subagent_output).`
}

func (s *SpawnSubagent) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task": map[string]any{
				"type":        "string",
				"description": "Task for the sub-agent (alias: prompt)",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Grok-compatible alias for task",
			},
			"description": map[string]any{
				"type":        "string",
				"description": "Short 3–5 word label for the subtask",
			},
			"mode": map[string]any{
				"type":        "string",
				"description": "Legacy alias for subagent_type: explore | general | plan",
			},
			"subagent_type": map[string]any{
				"type":        "string",
				"description": "explore | plan | general-purpose (default explore)",
			},
			"capability_mode": map[string]any{
				"type":        "string",
				"description": "read-only | read-write | execute | all",
			},
			"isolation": map[string]any{
				"type":        "string",
				"description": "none (default) | worktree",
			},
			"persona": map[string]any{
				"type":        "string",
				"description": "Optional persona name",
			},
			"max_iterations": map[string]any{"type": "integer"},
			"background": map[string]any{
				"type":        "boolean",
				"description": "If true, return job id immediately; poll get_subagent_output",
			},
			"resume_from": map[string]any{
				"type":        "string",
				"description": "Finished subagent id to continue with same context + new prompt",
			},
		},
	}
}

type spawnInput struct {
	Task           string `json:"task"`
	Prompt         string `json:"prompt"`
	Description    string `json:"description"`
	Mode           string `json:"mode"`
	SubagentType   string `json:"subagent_type"`
	CapabilityMode string `json:"capability_mode"`
	Isolation      string `json:"isolation"`
	Persona        string `json:"persona"`
	MaxIterations  int    `json:"max_iterations"`
	Background     bool   `json:"background"`
	ResumeFrom     string `json:"resume_from"`
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
		task = strings.TrimSpace(in.Prompt)
	}
	if task == "" && strings.TrimSpace(in.ResumeFrom) == "" {
		return Result{Error: "task or prompt required (or resume_from with follow-up prompt)"}
	}
	if task == "" && strings.TrimSpace(in.ResumeFrom) != "" {
		return Result{Error: "resume_from requires a new task/prompt to continue"}
	}
	if SubagentRunner == nil {
		return Result{Error: "subagent runner not wired"}
	}

	// Resume path: inherit type/system/messages from prior job
	var prior *SubJob
	if rid := strings.TrimSpace(in.ResumeFrom); rid != "" {
		j, ok := SubJobs.Get(rid)
		if !ok {
			return Result{Error: "resume_from: unknown subagent id " + rid}
		}
		if j.Status == SubRunning {
			return Result{Error: "resume_from: subagent still running — wait or cancel"}
		}
		prior = &j
		// inherit defaults when not overridden
		if strings.TrimSpace(in.SubagentType) == "" && strings.TrimSpace(in.Mode) == "" {
			in.SubagentType = j.AgentType
		}
		if strings.TrimSpace(in.Persona) == "" && j.Persona != "" {
			in.Persona = j.Persona
		}
		if strings.TrimSpace(in.Description) == "" {
			in.Description = "resume " + j.ID
		}
	}

	agentType := resolveSubagentType(in.SubagentType, in.Mode)
	capMode := ParseCapabilityMode(in.CapabilityMode)
	if strings.TrimSpace(in.CapabilityMode) == "" {
		switch agentType {
		case "explore", "plan":
			capMode = CapReadOnly
		default:
			capMode = CapAll
		}
	}

	isolation := strings.ToLower(strings.TrimSpace(in.Isolation))
	if isolation == "" {
		isolation = "none"
	}

	var persona *personas.Persona
	if name := strings.TrimSpace(in.Persona); name != "" {
		p, ok := personas.Global().Get(name)
		if !ok {
			return Result{Error: "unknown persona: " + name + " (see /personas)"}
		}
		if strings.TrimSpace(p.Resolved) == "" && strings.TrimSpace(p.Instructions) == "" {
			return Result{Error: "persona has no instructions: " + name}
		}
		persona = p
		if isolation == "none" && strings.EqualFold(p.DefaultIsolation, "worktree") {
			isolation = "worktree"
		}
	}

	maxIter := in.MaxIterations
	if maxIter <= 0 {
		if prior != nil && prior.MaxIterations > 0 {
			maxIter = prior.MaxIterations
		} else {
			maxIter = 6
		}
	}
	if maxIter > 16 {
		maxIter = 16
	}

	spec := runSpec{
		ParentWorkDir: s.WorkDir,
		Task:          task,
		Description:   in.Description,
		AgentType:     agentType,
		CapMode:       capMode,
		Isolation:     isolation,
		Persona:       persona,
		MaxIter:       maxIter,
		Prior:         prior,
	}

	if in.Background {
		// Capture the manager pointer for the background goroutine. Tests (and
		// rare runtime rebinds) may replace the package-level SubJobs var; the
		// in-flight job must keep using the manager it was registered on.
		mgr := SubJobs
		id := mgr.AllocID()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
		job := &SubJob{
			ID:            id,
			Description:   in.Description,
			AgentType:     agentType,
			Isolation:     isolation,
			Status:        SubRunning,
			Started:       time.Now(),
			MaxIterations: maxIter,
			cancel:        cancel,
		}
		if persona != nil {
			job.Persona = persona.Name
		}
		if job.Description == "" {
			job.Description = agentType
		}
		mgr.Put(job)

		go func() {
			defer cancel()
			res := executeSubagentRun(ctx, spec, nil)
			mgr.update(id, func(j *SubJob) {
				j.Ended = time.Now()
				j.Output = res.output
				j.Error = res.errStr
				j.ToolsUsed = res.toolsUsed
				j.WorkDir = res.workdir
				j.System = res.system
				j.Messages = res.messages
				if res.errStr != "" {
					j.Status = SubFailed
				} else if res.cancelled {
					j.Status = SubCancelled
				} else {
					j.Status = SubSucceeded
				}
				j.cancel = nil
			})
			if j, ok := mgr.Get(id); ok {
				mgr.notify(&j)
			}
		}()

		return Result{
			Success: true,
			Output: fmt.Sprintf(
				"Background subagent %s started (%s).\nPoll: get_subagent_output id=%s\nOr: /subagents show %s\nCancel: /subagents cancel %s",
				id, agentType, id, id, id,
			),
		}
	}

	// Synchronous run
	if progress != nil {
		msg := "subagent (" + agentType + ")"
		if in.Description != "" {
			msg = "subagent (" + in.Description + ")"
		}
		if persona != nil {
			msg += " persona=" + persona.Name
		}
		if isolation == "worktree" {
			msg += " [worktree]"
		}
		if prior != nil {
			msg += " resume=" + prior.ID
		}
		progress(msg + " starting…")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	res := executeSubagentRun(ctx, spec, progress)
	id := SubJobs.AllocID()
	job := &SubJob{
		ID: id, Description: in.Description, AgentType: agentType,
		Isolation: isolation, WorkDir: res.workdir, System: res.system,
		MaxIterations: maxIter, ToolsUsed: res.toolsUsed,
		Started: time.Now().Add(-time.Second), Ended: time.Now(),
		Messages: res.messages, Output: res.output, Error: res.errStr,
	}
	if persona != nil {
		job.Persona = persona.Name
	}
	if job.Description == "" {
		job.Description = agentType
	}
	if res.errStr != "" {
		job.Status = SubFailed
	} else if res.cancelled {
		job.Status = SubCancelled
	} else {
		job.Status = SubSucceeded
	}
	SubJobs.Put(job)

	if res.errStr != "" {
		return Result{Error: "subagent: " + res.errStr, Output: res.output}
	}
	header := fmt.Sprintf("Subagent %s type=%s cap=%s isolation=%s tools=%d",
		id, agentType, capMode, isolation, res.toolsUsed)
	if persona != nil {
		header += " persona=" + persona.Name
	}
	if prior != nil {
		header += " resumed_from=" + prior.ID
	}
	if res.wtNote != "" {
		header += "\n" + res.wtNote
	}
	header += "\n\n"
	return Result{Success: true, Output: header + res.output}
}

type runSpec struct {
	ParentWorkDir string
	Task          string
	Description   string
	AgentType     string
	CapMode       CapabilityMode
	Isolation     string
	Persona       *personas.Persona
	MaxIter       int
	Prior         *SubJob
}

type runResult struct {
	output    string
	errStr    string
	toolsUsed int
	workdir   string
	system    string
	messages  []provider.Message
	wtNote    string
	cancelled bool
}

func executeSubagentRun(ctx context.Context, spec runSpec, progress ProgressFunc) runResult {
	workdir := spec.ParentWorkDir
	var wt *WorktreeSession
	var wtNote string
	if spec.Isolation == "worktree" {
		label := spec.Description
		if label == "" {
			label = spec.AgentType
		}
		sess, err := CreateWorktree(spec.ParentWorkDir, label)
		if err != nil {
			return runResult{errStr: "isolation worktree: " + err.Error()}
		}
		wt = sess
		workdir = sess.Path
		defer wt.Cleanup()
		wtNote = "worktree=" + wt.Path + " branch=" + wt.Branch +
			"\n(worktree cleaned up after run — commit inside worktree if you need to keep changes)"
	}

	tools := buildSubagentTools(spec.AgentType, spec.CapMode, workdir)
	sys := buildSubagentSystem(spec.AgentType, spec.Persona, spec.Description)
	if spec.Prior != nil && strings.TrimSpace(spec.Prior.System) != "" {
		sys = spec.Prior.System + "\n\n# Resumed subagent\nContinue from prior work with the new user message."
	}

	var msgs []provider.Message
	if spec.Prior != nil && len(spec.Prior.Messages) > 0 {
		msgs = append(msgs, spec.Prior.Messages...)
	}
	msgs = append(msgs, provider.Message{Role: provider.RoleUser, Content: spec.Task})

	var text strings.Builder
	toolsUsed := 0
	var lastErr string
	cancelled := false
	SubagentRunner(ctx, workdir, sys, msgs, tools, spec.MaxIter, func(ev SubagentEvent) {
		switch ev.Kind {
		case "thinking":
			// fold reasoning into progress only (not final summary body)
			if progress != nil && ev.Text != "" {
				progress("thinking: " + truncateRunes(ev.Text, 60))
			}
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
	if ctx.Err() == context.Canceled {
		cancelled = true
	} else if ctx.Err() == context.DeadlineExceeded {
		lastErr = "timed out after 4m"
	}

	out := strings.TrimSpace(text.String())
	if out == "" {
		out = "(subagent finished with no text)"
	}

	finalMsgs := append([]provider.Message{}, msgs...)
	finalMsgs = append(finalMsgs, provider.Message{Role: provider.RoleAssistant, Content: out})

	return runResult{
		output: out, errStr: lastErr, toolsUsed: toolsUsed,
		workdir: workdir, system: sys, messages: finalMsgs,
		wtNote: wtNote, cancelled: cancelled,
	}
}

func resolveSubagentType(subType, mode string) string {
	t := strings.ToLower(strings.TrimSpace(subType))
	if t == "" {
		t = strings.ToLower(strings.TrimSpace(mode))
	}
	switch t {
	case "", "explore", "research", "ro":
		return "explore"
	case "plan", "planner", "design":
		return "plan"
	case "general", "general-purpose", "general_purpose", "full", "act":
		return "general-purpose"
	default:
		return "explore"
	}
}

func buildSubagentTools(agentType string, cap CapabilityMode, workdir string) *Registry {
	parent := SubagentParentRegistry
	switch agentType {
	case "plan":
		if cap == CapReadOnly || cap == CapAll {
			return NewPlanRegistry(workdir, parent)
		}
		return FilterRegistryByCapability(NewPlanRegistry(workdir, parent), cap, workdir, parent)
	case "explore":
		return FilterRegistryByCapability(nil, CapReadOnly, workdir, parent)
	default:
		// Prefer parent registry so MCP/research and staged write mode stay aligned.
		var base *Registry
		if parent != nil {
			base = parent.CloneWithoutSpawn()
			// Re-bind workdir-sensitive tools if child workdir differs (worktree).
			if workdir != "" && parent.GetStagedWriter() != nil {
				// keep same staged writer mode; tools already share parent workdir
				// for isolation=worktree, rebuild full registry under child path
				if sw := parent.GetStagedWriter(); sw != nil && workdir != sw.WorkDirSafe() {
					base = NewRegistry(workdir)
					// copy MCP tools from parent
					for _, t := range parent.List() {
						if strings.HasPrefix(t.Name(), "mcp_") {
							base.Register(t)
						}
					}
					// sync write mode
					if nsw := base.GetStagedWriter(); nsw != nil {
						nsw.SetMode(sw.Mode())
					}
					// strip spawn
					base = cloneWithoutSpawn(base)
				}
			}
		} else {
			base = NewRegistry(workdir)
			base = cloneWithoutSpawn(base)
		}
		return FilterRegistryByCapability(base, cap, workdir, parent)
	}
}

func buildSubagentSystem(agentType string, persona *personas.Persona, desc string) string {
	var b strings.Builder
	b.WriteString("You are a CodeForge sub-agent. Complete the task and return a concise summary.\n")
	b.WriteString("Do not ask the user questions. Prefer tools for evidence.\n")
	if desc != "" {
		b.WriteString("Task label: ")
		b.WriteString(desc)
		b.WriteByte('\n')
	}
	switch agentType {
	case "explore":
		b.WriteString("Mode EXPLORE: READ-ONLY. Do not edit project files. Search and summarize.\n")
	case "plan":
		b.WriteString("Mode PLAN: Explore the codebase, then produce a structured implementation plan.\n")
		b.WriteString("Use write_plan for the plan document. Do NOT edit project source files.\n")
		b.WriteString("End with a clear plan summary in your final message.\n")
	case "general-purpose":
		b.WriteString("Mode GENERAL: You may edit files and run commands as needed. Be careful and verify.\n")
	}
	if persona != nil {
		if rem := persona.SystemReminder(); rem != "" {
			b.WriteByte('\n')
			b.WriteString(rem)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
