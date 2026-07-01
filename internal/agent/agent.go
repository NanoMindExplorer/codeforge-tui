// Package agent implements CodeForge's tool-calling agent loop: it sends
// the conversation to an AI provider, and whenever the model requests one
// or more tool calls, executes them via the tool registry, feeds the
// results back, and repeats until the model produces a final answer, the
// iteration limit is reached, or the context is cancelled.
//
// This corresponds to "Lapisan 3: Agent & Workflow Engine" in the product
// plan. Tool calling requires non-streaming requests (the underlying
// Claude and Gemini APIs don't support mixing streaming with tool use), so
// the loop uses Provider.Complete rather than Provider.Stream. Progress is
// still reported incrementally to the caller via the returned event
// channel, so the UI can show each step as it happens.
package agent

import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/codeforge/tui/internal/provider"
    "github.com/codeforge/tui/internal/tool"
)

// DefaultMaxIterations bounds how many times the loop will call the model
// in a single turn before giving up, preventing a runaway agent.
const DefaultMaxIterations = 8

// DefaultMaxTokens is used when Config.MaxTokens is unset.
const DefaultMaxTokens = 4096

type EventKind int

const (
    // EventText carries assistant-visible text produced by the model.
    EventText EventKind = iota
    // EventToolCall fires when the model requests a tool call, before it
    // has executed.
    EventToolCall
    // EventToolResult fires after a tool call has finished executing.
    EventToolResult
    // EventDone fires exactly once, when the loop has finished
    // successfully (with or without tool calls along the way).
    EventDone
    // EventError fires when the loop terminates abnormally (provider
    // error, unknown tool, or iteration limit reached).
    EventError
)

type Event struct {
    Kind EventKind

    Text string // EventText

    ToolName    string // EventToolCall, EventToolResult
    ToolInput   string // EventToolCall: raw JSON arguments
    ToolOutput  string // EventToolResult: human-readable result
    ToolSuccess bool   // EventToolResult
    ToolDiff    string // EventToolResult: unified diff, if any (e.g. write_file)

    InputTokens  int // EventDone
    OutputTokens int // EventDone

    Error error // EventError
}

type Config struct {
    Provider      provider.Provider
    Tools         *tool.Registry
    System        string
    MaxTokens     int
    Temperature   float64
    MaxIterations int
}

// Run starts the agent loop in a goroutine and returns a channel of events
// describing its progress. The channel is closed when the loop finishes.
// history is the conversation so far (not including a not-yet-sent user
// turn — callers append the new user message before calling Run).
func Run(ctx context.Context, cfg Config, history []provider.Message) <-chan Event {
    out := make(chan Event, 32)

    go func() {
        defer close(out)
        runLoop(ctx, cfg, history, out)
    }()

    return out
}

func runLoop(ctx context.Context, cfg Config, history []provider.Message, out chan<- Event) {
    if cfg.Provider == nil {
        out <- Event{Kind: EventError, Error: fmt.Errorf("no AI provider configured")}
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

    toolDefs := buildToolDefs(cfg.Tools)

    messages := make([]provider.Message, len(history))
    copy(messages, history)

    for iter := 0; iter < maxIter; iter++ {
        if ctx.Err() != nil {
            out <- Event{Kind: EventDone}
            return
        }

        req := provider.CompletionRequest{
            Messages:    messages,
            MaxTokens:   maxTokens,
            Temperature: cfg.Temperature,
            System:      cfg.System,
            Tools:       toolDefs,
        }

        resp, err := cfg.Provider.Complete(ctx, req)
        if err != nil {
            if ctx.Err() != nil {
                out <- Event{Kind: EventDone}
                return
            }
            out <- Event{Kind: EventError, Error: err}
            return
        }

        if resp.Content != "" {
            out <- Event{Kind: EventText, Text: resp.Content}
        }

        if len(resp.ToolCalls) == 0 {
            out <- Event{Kind: EventDone, InputTokens: resp.InputTokens, OutputTokens: resp.OutputTokens}
            return
        }

        messages = append(messages, provider.Message{
            Role:      provider.RoleAssistant,
            Content:   resp.Content,
            ToolCalls: resp.ToolCalls,
        })

        for _, call := range resp.ToolCalls {
            out <- Event{Kind: EventToolCall, ToolName: call.Name, ToolInput: call.Input}

            result := executeTool(cfg.Tools, call)

            out <- Event{
                Kind:        EventToolResult,
                ToolName:    call.Name,
                ToolOutput:  result.summary,
                ToolSuccess: result.success,
                ToolDiff:    result.diff,
            }

            messages = append(messages, provider.Message{
                Role:       provider.RoleTool,
                Content:    result.forModel,
                ToolCallID: call.ID,
                ToolName:   call.Name,
                IsError:    !result.success,
            })
        }

        if iter == maxIter-1 {
            out <- Event{Kind: EventError, Error: fmt.Errorf("reached max iterations (%d) without a final answer", maxIter)}
            return
        }
    }
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
    summary  string // shown to the user in the chat transcript
    forModel string // sent back to the LLM as the tool_result content
    diff     string
}

func executeTool(reg *tool.Registry, call provider.ToolCall) toolExecResult {
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

    res := t.Execute(input)
    if !res.Success {
        msg := res.Error
        if msg == "" {
            msg = "tool execution failed"
        }
        return toolExecResult{success: false, summary: msg, forModel: msg, diff: res.Diff}
    }
    return toolExecResult{success: true, summary: res.Output, forModel: res.Output, diff: res.Diff}
}
