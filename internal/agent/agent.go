// Package agent implements CodeForge's tool-calling agent loop: it sends
// the conversation to an AI provider, and whenever the model requests one
// or more tool calls, executes them via the tool registry, feeds the
// results back, and repeats until the model produces a final answer, the
// iteration limit is reached, or the context is cancelled.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/redact"
	"github.com/codeforge/tui/internal/tool"
)

const DefaultMaxIterations = 12
const DefaultMaxTokens = 4096
const defaultRateLimitRetries = 1
const defaultRateLimitBackoff = 1 * time.Second

type EventKind int

const (
	EventText EventKind = iota
	// EventThinking is native provider reasoning / chain-of-thought (not assistant answer text).
	EventThinking
	EventToolCall
	EventToolResult
	// EventToolProgress is streamed mid-tool for long-running operations.
	EventToolProgress
	EventDone
	EventError
	// EventInfo is a non-fatal system notice (e.g. reasoning auto-retry).
	EventInfo
)

type Event struct {
	Kind EventKind

	Text string
	// Thinking holds native reasoning token text (EventThinking).
	Thinking string

	ToolName    string
	ToolInput   string
	ToolOutput  string
	ToolSuccess bool
	ToolDiff    string
	// Progress is a partial chunk for EventToolProgress.
	Progress string

	InputTokens     int
	OutputTokens    int
	ReasoningTokens int

	Error error
}

// Authorizer gates tool execution (Phase 6 permissions).
// Return nil to allow; non-nil error is shown to the model as a tool failure.
type Authorizer interface {
	Authorize(ctx context.Context, toolName, input string) error
	NotifyPost(ctx context.Context, toolName, input, output string, success bool)
}

type Config struct {
	Provider      provider.Provider
	Tools         *tool.Registry
	System        string
	MaxTokens     int
	Temperature   float64
	MaxIterations int
	// Auth optional permission/hooks gate
	Auth Authorizer
	// RateLimitRetries is extra attempts after a rate_limit error (default 1).
	// Set 0 to disable rate-limit retry.
	RateLimitRetries *int
	// Sleep is used for rate-limit backoff (injectable in tests).
	// Default: interruptible sleep that returns on ctx cancel.
	Sleep func(ctx context.Context, d time.Duration) error
}

func Run(ctx context.Context, cfg Config, history []provider.Message) <-chan Event {
	out := make(chan Event, 64)
	go func() {
		defer close(out)
		runLoop(ctx, cfg, history, out)
	}()
	return out
}

func emitErr(out chan<- Event, err error) {
	if err == nil {
		return
	}
	// Keep LoopError as-is so headless can map code=max_iterations|canceled|no_provider.
	// FormatUserError understands UserMessage() interface and ProviderError.
	out <- Event{Kind: EventError, Error: err}
}

func runLoop(ctx context.Context, cfg Config, history []provider.Message, out chan<- Event) {
	if cfg.Provider == nil {
		emitErr(out, errNoProvider())
		return
	}

	maxIter := cfg.MaxIterations
	if maxIter <= 0 {
		maxIter = DefaultMaxIterations
	}
	maxTokens := cfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	}
	sleep := cfg.Sleep
	if sleep == nil {
		sleep = defaultSleep
	}
	rlRetries := defaultRateLimitRetries
	if cfg.RateLimitRetries != nil {
		rlRetries = *cfg.RateLimitRetries
	}

	toolDefs := buildToolDefs(cfg.Tools)
	messages := make([]provider.Message, len(history))
	copy(messages, history)

	for iter := 0; iter < maxIter; iter++ {
		if ctx.Err() != nil {
			emitErr(out, errCanceled())
			return
		}

		req := provider.CompletionRequest{
			Messages:    messages,
			MaxTokens:   maxTokens,
			Temperature: cfg.Temperature,
			System:      cfg.System,
			Tools:       toolDefs,
		}

		resp, notices, err := completeWithRetries(ctx, cfg.Provider, req, rlRetries, sleep)
		for _, n := range notices {
			out <- Event{Kind: EventInfo, Text: n}
		}
		if err != nil {
			if ctx.Err() != nil {
				emitErr(out, errCanceled())
				return
			}
			emitErr(out, err)
			return
		}

		if strings.TrimSpace(resp.Reasoning) != "" {
			out <- Event{Kind: EventThinking, Thinking: resp.Reasoning, ReasoningTokens: resp.ReasoningTokens}
		}
		if resp.Content != "" {
			out <- Event{Kind: EventText, Text: resp.Content}
		}

		if len(resp.ToolCalls) == 0 {
			out <- Event{
				Kind: EventDone, InputTokens: resp.InputTokens, OutputTokens: resp.OutputTokens,
				ReasoningTokens: resp.ReasoningTokens,
			}
			return
		}

		messages = append(messages, provider.Message{
			Role:      provider.RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			out <- Event{Kind: EventToolCall, ToolName: call.Name, ToolInput: call.Input}

			// Permission / hooks gate before execution
			if cfg.Auth != nil {
				if err := cfg.Auth.Authorize(ctx, call.Name, call.Input); err != nil {
					msg := err.Error()
					out <- Event{
						Kind:        EventToolResult,
						ToolName:    call.Name,
						ToolOutput:  "🚫 " + msg,
						ToolSuccess: false,
					}
					messages = append(messages, provider.Message{
						Role:       provider.RoleTool,
						Content:    "Permission denied: " + msg,
						ToolCallID: call.ID,
						ToolName:   call.Name,
						IsError:    true,
					})
					continue
				}
			}

			result := executeTool(ctx, cfg.Tools, call, func(chunk string) {
				if chunk == "" {
					return
				}
				select {
				case out <- Event{Kind: EventToolProgress, ToolName: call.Name, Progress: chunk}:
				case <-ctx.Done():
				}
			})

			if cfg.Auth != nil {
				cfg.Auth.NotifyPost(ctx, call.Name, call.Input, result.summary, result.success)
			}

			out <- Event{
				Kind:        EventToolResult,
				ToolName:    call.Name,
				ToolOutput:  result.summary,
				ToolSuccess: result.success,
				ToolDiff:    result.diff,
			}

			// Cap + redact model-facing tool results (Q1.6)
			forModel := redact.Redact(result.forModel)
			if len(forModel) > 24_000 {
				forModel = forModel[:24_000] + "\n… (truncated for model context)"
			}

			messages = append(messages, provider.Message{
				Role:       provider.RoleTool,
				Content:    forModel,
				ToolCallID: call.ID,
				ToolName:   call.Name,
				IsError:    !result.success,
			})
		}

		if iter == maxIter-1 {
			emitErr(out, errMaxIterations(maxIter))
			return
		}
	}
}

// completeWithRetries: reasoning unsupported retry + one rate_limit backoff retry.
func completeWithRetries(
	ctx context.Context,
	p provider.Provider,
	req provider.CompletionRequest,
	rateLimitRetries int,
	sleep func(context.Context, time.Duration) error,
) (*provider.CompletionResponse, []string, error) {
	var notices []string

	resp, reasoningRetried, err := provider.CompleteRetryingReasoning(ctx, p, req)
	if reasoningRetried && err == nil {
		notices = append(notices, "🧠 Reasoning not supported by this model — continued without thinking\n  → set CODEFORGE_REASONING=off to skip the retry, or /model another id")
	}
	if err == nil {
		return resp, notices, nil
	}
	if ctx.Err() != nil {
		return nil, notices, err
	}

	// Rate-limit: wait and retry once (or RateLimitRetries times)
	for attempt := 0; attempt < rateLimitRetries; attempt++ {
		pe, ok := provider.AsProviderError(err)
		if !ok || pe == nil || pe.Code != provider.ErrRateLimit || !pe.Retry {
			break
		}
		wait := pe.RetryAfter
		if wait <= 0 {
			wait = defaultRateLimitBackoff
		}
		// Cap absurd retry-after values for UX
		if wait > 60*time.Second {
			wait = 60 * time.Second
		}
		notices = append(notices, fmt.Sprintf("⏳ Rate limited — retrying in ~%s (attempt %d/%d)", wait.Round(time.Second), attempt+1, rateLimitRetries))
		if err := sleep(ctx, wait); err != nil {
			return nil, notices, errCanceled()
		}
		// After rate limit, prefer reasoning off on retry to reduce load
		req2 := req
		req2.Reasoning = "off"
		resp, reasoningRetried, err = provider.CompleteRetryingReasoning(ctx, p, req2)
		if reasoningRetried && err == nil {
			notices = append(notices, "🧠 Reasoning not supported by this model — continued without thinking")
		}
		if err == nil {
			return resp, notices, nil
		}
		if ctx.Err() != nil {
			return nil, notices, errCanceled()
		}
	}
	return nil, notices, err
}

func buildToolDefs(reg *tool.Registry) []provider.ToolDefinition {
	if reg == nil {
		return nil
	}
	tools := reg.List()
	if len(tools) == 0 {
		return nil
	}
	defs := make([]provider.ToolDefinition, 0, len(tools))
	for _, t := range tools {
		defs = append(defs, provider.ToolDefinition{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		})
	}
	return defs
}

type toolExecResult struct {
	success  bool
	summary  string
	forModel string
	diff     string
}

func executeTool(ctx context.Context, reg *tool.Registry, call provider.ToolCall, progress tool.ProgressFunc) toolExecResult {
	if reg == nil {
		msg := "no tool registry configured"
		return toolExecResult{success: false, summary: msg, forModel: msg}
	}
	t, ok := reg.Get(call.Name)
	if !ok {
		msg := fmt.Sprintf("unknown tool: %s", call.Name)
		return toolExecResult{success: false, summary: msg, forModel: msg}
	}

	input := json.RawMessage(call.Input)
	if len(input) == 0 {
		input = json.RawMessage(`{}`)
	}

	// Prefer streaming executor when available
	if st, ok := t.(tool.StreamingExecutor); ok {
		res := st.ExecuteStream(input, progress)
		return mapResult(res)
	}

	// Shell: stream by running and emitting a start progress marker
	if progress != nil {
		progress(fmt.Sprintf("running %s…", call.Name))
	}
	res := t.Execute(input)
	// Emit trailing progress for long outputs
	if progress != nil && res.Success && len(res.Output) > 400 {
		// stream in chunks for UI
		chunks := chunkString(res.Output, 500)
		for i, ch := range chunks {
			if i >= 6 {
				progress("… (more output in result)")
				break
			}
			progress(ch)
		}
	}
	_ = ctx
	return mapResult(res)
}

func mapResult(res tool.Result) toolExecResult {
	if !res.Success {
		msg := res.Error
		if msg == "" {
			msg = "tool execution failed"
		}
		return toolExecResult{success: false, summary: msg, forModel: msg, diff: res.Diff}
	}
	return toolExecResult{success: true, summary: res.Output, forModel: res.Output, diff: res.Diff}
}

func chunkString(s string, n int) []string {
	var out []string
	for len(s) > 0 {
		if len(s) <= n {
			out = append(out, s)
			break
		}
		// break on newline when possible
		cut := n
		if i := strings.LastIndex(s[:n], "\n"); i > n/2 {
			cut = i + 1
		}
		out = append(out, s[:cut])
		s = s[cut:]
	}
	return out
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
