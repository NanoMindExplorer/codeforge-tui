// Package headless runs CodeForge agent without a TUI (CI / scripts).
package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/app"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/session"
)

// Options for a headless agent run.
type Options struct {
	Task           string
	JSON           bool
	Act            bool
	Plan           bool
	AlwaysApprove  bool // YOLO / always_approve
	DontAsk        bool // deny anything that would prompt
	MaxIter        int
	Timeout        time.Duration
	WorkDir        string
	Quiet          bool
	SystemExtra    string
}

// Result is the structured outcome of a headless run.
type Result struct {
	OK           bool              `json:"ok"`
	Text         string            `json:"text"`
	Error        string            `json:"error,omitempty"`
	ToolCalls    int               `json:"tool_calls"`
	Tools        []string          `json:"tools,omitempty"`
	InputTokens  int               `json:"input_tokens,omitempty"`
	OutputTokens int               `json:"output_tokens,omitempty"`
	DurationMs   int64             `json:"duration_ms"`
	Events       []EventRecord     `json:"events,omitempty"`
	WorkDir      string            `json:"workdir"`
	Provider     string            `json:"provider,omitempty"`
	Model        string            `json:"model,omitempty"`
	SessionID    string            `json:"session_id,omitempty"`
	Meta         map[string]string `json:"meta,omitempty"`
}

// EventRecord is a simplified event for JSON streams.
type EventRecord struct {
	Kind    string `json:"kind"`
	Tool    string `json:"tool,omitempty"`
	Text    string `json:"text,omitempty"`
	Success *bool  `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Run executes the agent and writes human or JSON output to w.
func Run(opt Options, w io.Writer) (Result, error) {
	start := time.Now()
	if strings.TrimSpace(opt.Task) == "" {
		return Result{OK: false, Error: "task required"}, fmt.Errorf("task required")
	}
	if opt.Timeout <= 0 {
		opt.Timeout = 10 * time.Minute
	}
	if opt.MaxIter <= 0 {
		opt.MaxIter = 12
	}

	rt, err := app.Bootstrap(app.Options{
		WorkDir:   opt.WorkDir,
		Quiet:     opt.Quiet || opt.JSON,
		ActMode:   opt.Act || !opt.Plan,
		PlanMode:  opt.Plan,
		SkipIndex: false,
	})
	if err != nil {
		return Result{OK: false, Error: err.Error()}, err
	}
	if rt.Tele != nil {
		rt.Tele.Event("headless_agent", map[string]any{"json": opt.JSON})
		defer rt.Tele.Flush()
	}

	p, err := rt.ProvReg.Current()
	if err != nil {
		return Result{OK: false, Error: err.Error()}, err
	}
	if err := p.ValidateConfig(); err != nil {
		return Result{OK: false, Error: err.Error()}, err
	}

	sys := `You are CodeForge headless agent (CI/scripts). Be concise and complete the task.
Prefer search_replace/apply_patch over full rewrites. Run diagnostics after edits.
Reply with a clear summary of what you did.`
	sys = rules.Inject(sys, rt.Rules)
	if opt.SystemExtra != "" {
		sys += "\n\n" + opt.SystemExtra
	}

	msgs := []provider.Message{{Role: provider.RoleUser, Content: opt.Task}}
	ctx, cancel := context.WithTimeout(context.Background(), opt.Timeout)
	defer cancel()

	// Phase 6: permission gate (headless — no interactive ask)
	eng := permission.FromConfig(rt.Cfg, rt.WorkDir)
	eng.Headless = true
	if opt.AlwaysApprove || opt.Act {
		eng.SetMode(permission.ModeAlwaysApprove)
	}
	if opt.DontAsk {
		eng.SetMode(permission.ModeDontAsk)
	}
	if opt.Plan {
		eng.SetMode(permission.ModePlan)
	}

	ch := agent.Run(ctx, agent.Config{
		Provider:      p,
		Tools:         rt.ToolReg,
		System:        sys,
		MaxTokens:     4096,
		MaxIterations: opt.MaxIter,
		Auth:          eng,
	}, msgs)

	var (
		text      strings.Builder
		tools     []string
		toolCalls int
		events    []EventRecord
		inTok     int
		outTok    int
		lastErr   error
	)

	for ev := range ch {
		switch ev.Kind {
		case agent.EventText:
			text.WriteString(ev.Text)
			if !opt.JSON && !opt.Quiet {
				fmt.Fprint(w, ev.Text)
			}
			events = append(events, EventRecord{Kind: "text", Text: ev.Text})
		case agent.EventToolCall:
			toolCalls++
			tools = append(tools, ev.ToolName)
			if !opt.JSON && !opt.Quiet {
				fmt.Fprintf(w, "\n🔧 %s\n", ev.ToolName)
			}
			events = append(events, EventRecord{Kind: "tool_call", Tool: ev.ToolName})
		case agent.EventToolProgress:
			if !opt.JSON && !opt.Quiet && ev.Progress != "" {
				fmt.Fprintf(w, "⋯ %s\n", trim(ev.Progress, 120))
			}
			events = append(events, EventRecord{Kind: "progress", Text: ev.Progress, Tool: ev.ToolName})
		case agent.EventToolResult:
			ok := ev.ToolSuccess
			if !opt.JSON && !opt.Quiet {
				icon := "✓"
				if !ok {
					icon = "✗"
				}
				fmt.Fprintf(w, "%s %s: %s\n", icon, ev.ToolName, trim(ev.ToolOutput, 160))
			}
			events = append(events, EventRecord{Kind: "tool_result", Tool: ev.ToolName, Success: &ok, Text: trim(ev.ToolOutput, 500)})
		case agent.EventDone:
			inTok, outTok = ev.InputTokens, ev.OutputTokens
			events = append(events, EventRecord{Kind: "done"})
		case agent.EventError:
			lastErr = ev.Error
			events = append(events, EventRecord{Kind: "error", Error: ev.Error.Error()})
		}
	}

	// Persist session (shared v2 layout with TUI)
	sess := session.New(rt.ProvReg.CurrentName(), p.Model(), rt.WorkDir)
	sess.Messages = []provider.Message{
		{Role: provider.RoleUser, Content: opt.Task},
		{Role: provider.RoleAssistant, Content: strings.TrimSpace(text.String())},
	}
	sess.Tokens = inTok + outTok
	_, _ = sess.RecordRewindPoint(opt.Task, "headless")
	_ = sess.Save()
	_ = sess.AppendEvent("headless_done", map[string]any{
		"ok": lastErr == nil, "tools": toolCalls, "ms": time.Since(start).Milliseconds(),
	})

	res := Result{
		OK:           lastErr == nil,
		Text:         strings.TrimSpace(text.String()),
		ToolCalls:    toolCalls,
		Tools:        uniq(tools),
		InputTokens:  inTok,
		OutputTokens: outTok,
		DurationMs:   time.Since(start).Milliseconds(),
		WorkDir:      rt.WorkDir,
		Provider:     rt.ProvReg.CurrentName(),
		Model:        p.Model(),
		SessionID:    sess.ID,
	}
	if lastErr != nil {
		res.Error = lastErr.Error()
	}
	if opt.JSON {
		res.Events = events
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	} else if !opt.Quiet {
		if res.Text != "" && !strings.HasSuffix(res.Text, "\n") {
			fmt.Fprintln(w)
		}
		if lastErr != nil {
			fmt.Fprintf(w, "\n⚠ agent error: %v\n", lastErr)
		}
		fmt.Fprintf(w, "\n— done in %dms · tools=%d · tokens in/out=%d/%d · session=%s\n",
			res.DurationMs, res.ToolCalls, res.InputTokens, res.OutputTokens, sess.ID)
	}

	if lastErr != nil {
		return res, lastErr
	}
	return res, nil
}

// RunCLI is a convenience that prints to stdout and sets exit-friendly error.
func RunCLI(opt Options) int {
	res, err := Run(opt, os.Stdout)
	if err != nil || !res.OK {
		return 1
	}
	return 0
}

func trim(s string, n int) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
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
