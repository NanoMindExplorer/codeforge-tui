package provider

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "github.com/liushuangls/go-anthropic/v2"
)

type ClaudeProvider struct {
    client *anthropic.Client
    model  string
    apiKey string
}

func NewClaudeProvider(apiKey, defaultModel string) *ClaudeProvider {
    if apiKey == "" {
        apiKey = os.Getenv("ANTHROPIC_API_KEY")
    }
    cp := &ClaudeProvider{
        apiKey: apiKey,
        model:  defaultModel,
    }
    if apiKey != "" {
        cp.client = anthropic.NewClient(apiKey)
    }
    return cp
}

func (p *ClaudeProvider) Name() string { return "claude" }

func (p *ClaudeProvider) Model() string { return p.model }

func (p *ClaudeProvider) SetModel(id string) error {
    for _, m := range p.Models() {
        if m.ID == id {
            p.model = id
            return nil
        }
    }
    // Allow unknown model IDs (forward-compat) if non-empty
    if id != "" {
        p.model = id
        return nil
    }
    return fmt.Errorf("model id required")
}

func (p *ClaudeProvider) Models() []ModelInfo {
    return []ModelInfo{
        {ID: "claude-sonnet-4-20250514", Name: "Claude Sonnet 4", ContextWindow: 200000, InputCost: 3.0, OutputCost: 15.0},
        {ID: "claude-opus-4-0-20250918", Name: "Claude Opus 4", ContextWindow: 200000, InputCost: 15.0, OutputCost: 75.0},
        {ID: "claude-haiku-4-20250414", Name: "Claude Haiku 4", ContextWindow: 200000, InputCost: 0.80, OutputCost: 4.0},
    }
}

func (p *ClaudeProvider) ValidateConfig() error {
    if p.apiKey == "" {
        return fmt.Errorf("ANTHROPIC_API_KEY not set")
    }
    if p.client == nil {
        return fmt.Errorf("anthropic client not initialized")
    }
    return nil
}

// toAnthropicMessages converts the provider-agnostic conversation into
// Anthropic's content-block message format. Consecutive role="tool"
// messages are grouped into a single user message with multiple
// tool_result blocks, since the Anthropic API requires all tool results
// for one assistant turn to arrive together.
func toAnthropicMessages(msgs []Message) []anthropic.Message {
    out := make([]anthropic.Message, 0, len(msgs))
    var pendingResults []anthropic.MessageContent

    flush := func() {
        if len(pendingResults) > 0 {
            out = append(out, anthropic.Message{Role: anthropic.RoleUser, Content: pendingResults})
            pendingResults = nil
        }
    }

    for _, m := range msgs {
        switch m.Role {
        case RoleTool:
            pendingResults = append(pendingResults,
                anthropic.NewToolResultMessageContent(m.ToolCallID, m.Content, m.IsError))

        case RoleAssistant:
            flush()
            var blocks []anthropic.MessageContent
            if m.Content != "" {
                blocks = append(blocks, anthropic.NewTextMessageContent(m.Content))
            }
            for _, tc := range m.ToolCalls {
                blocks = append(blocks, anthropic.NewToolUseMessageContent(tc.ID, tc.Name, json.RawMessage(tc.Input)))
            }
            if len(blocks) == 0 {
                blocks = append(blocks, anthropic.NewTextMessageContent(""))
            }
            out = append(out, anthropic.Message{Role: anthropic.RoleAssistant, Content: blocks})

        default: // "user" and anything else
            flush()
            out = append(out, anthropic.NewUserTextMessage(m.Content))
        }
    }
    flush()
    return out
}

func toAnthropicTools(defs []ToolDefinition) []anthropic.ToolDefinition {
    if len(defs) == 0 {
        return nil
    }
    out := make([]anthropic.ToolDefinition, 0, len(defs))
    for _, d := range defs {
        out = append(out, anthropic.ToolDefinition{
            Name:        d.Name,
            Description: d.Description,
            InputSchema: d.InputSchema,
        })
    }
    return out
}

func (p *ClaudeProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    if err := p.ValidateConfig(); err != nil {
        return nil, err
    }
    model := req.Model
    if model == "" {
        model = p.model
    }
    maxTokens := req.MaxTokens
    if maxTokens == 0 {
        maxTokens = 4096
    }
    anthropicReq := anthropic.MessagesRequest{
        Model:     anthropic.Model(model),
        Messages:  toAnthropicMessages(req.Messages),
        MaxTokens: maxTokens,
        Tools:     toAnthropicTools(req.Tools),
    }
    if req.System != "" {
        anthropicReq.System = req.System
    }
    // Temperature is incompatible with extended thinking on some models — skip when reasoning on.
    if req.Temperature > 0 && !req.WantsReasoning(model) {
        anthropicReq.SetTemperature(float32(req.Temperature))
    }
    if req.WantsReasoning(model) {
        budget := 4096
        if maxTokens > 8192 {
            budget = 8192
        }
        if budget >= maxTokens {
            budget = maxTokens / 2
        }
        if budget < 1024 {
            budget = 1024
            if maxTokens <= 1024 {
                maxTokens = 2048
                anthropicReq.MaxTokens = maxTokens
            }
        }
        anthropicReq.Thinking = &anthropic.Thinking{
            Type:         anthropic.ThinkingTypeEnabled,
            BudgetTokens: budget,
        }
    }
    resp, err := p.client.CreateMessages(ctx, anthropicReq)
    if err != nil {
        return nil, Classify(err, 0, err.Error(), "claude")
    }
    result := &CompletionResponse{
        InputTokens:  resp.Usage.InputTokens,
        OutputTokens: resp.Usage.OutputTokens,
        StopReason:   string(resp.StopReason),
    }
    for _, content := range resp.Content {
        switch content.Type {
        case anthropic.MessagesContentTypeText:
            result.Content += content.GetText()
        case anthropic.MessagesContentTypeThinking:
            if content.MessageContentThinking != nil {
                result.Reasoning += content.MessageContentThinking.Thinking
            }
        case anthropic.MessagesContentTypeToolUse:
            if content.MessageContentToolUse != nil {
                result.ToolCalls = append(result.ToolCalls, ToolCall{
                    ID:    content.MessageContentToolUse.ID,
                    Name:  content.MessageContentToolUse.Name,
                    Input: string(content.MessageContentToolUse.Input),
                })
            }
        }
    }
    return result, nil
}

// Stream provides plain text streaming and does not support tool calling
// (the Anthropic API itself rejects streaming requests that include
// tools). The agent loop uses Complete for turns where the model may call
// tools; Stream remains available for simple, tool-free chat.
func (p *ClaudeProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamToken, error) {
    if err := p.ValidateConfig(); err != nil {
        return nil, err
    }
    model := req.Model
    if model == "" {
        model = p.model
    }
    maxTokens := req.MaxTokens
    if maxTokens == 0 {
        maxTokens = 4096
    }
    anthropicReq := anthropic.MessagesRequest{
        Model:     anthropic.Model(model),
        Messages:  toAnthropicMessages(req.Messages),
        MaxTokens: maxTokens,
    }
    if req.System != "" {
        anthropicReq.System = req.System
    }
    if req.Temperature > 0 && !req.WantsReasoning(model) {
        anthropicReq.SetTemperature(float32(req.Temperature))
    }
    if req.WantsReasoning(model) {
        budget := 4096
        if budget >= maxTokens {
            budget = maxTokens / 2
        }
        if budget < 1024 {
            budget = 1024
            if maxTokens <= 1024 {
                anthropicReq.MaxTokens = 2048
            }
        }
        anthropicReq.Thinking = &anthropic.Thinking{
            Type:         anthropic.ThinkingTypeEnabled,
            BudgetTokens: budget,
        }
    }
    out := make(chan StreamToken, 100)
    go func() {
        defer close(out)
        var inputTokens, outputTokens int
        streamReq := anthropic.MessagesStreamRequest{
            MessagesRequest: anthropicReq,
            OnContentBlockDelta: func(data anthropic.MessagesEventContentBlockDeltaData) {
                // thinking_delta
                if data.Delta.MessageContentThinking != nil && data.Delta.MessageContentThinking.Thinking != "" {
                    out <- StreamToken{Reasoning: data.Delta.MessageContentThinking.Thinking}
                    return
                }
                if text := data.Delta.GetText(); text != "" {
                    out <- StreamToken{Text: text}
                }
            },
            OnMessageStart: func(data anthropic.MessagesEventMessageStartData) {
                inputTokens = data.Message.Usage.InputTokens
            },
            OnMessageDelta: func(data anthropic.MessagesEventMessageDeltaData) {
                outputTokens = data.Usage.OutputTokens
            },
            OnMessageStop: func(data anthropic.MessagesEventMessageStopData) {
                out <- StreamToken{
                    Done:         true,
                    InputTokens:  inputTokens,
                    OutputTokens: outputTokens,
                }
            },
            OnError: func(err anthropic.ErrorResponse) {
                msg := err.Type
                if err.Error != nil {
                    msg = err.Error.Message
                }
                out <- StreamToken{
                    Done:  true,
                    Error: Classify(nil, 0, msg, "claude"),
                }
            },
        }
        _, err := p.client.CreateMessagesStream(ctx, streamReq)
        if err != nil {
            out <- StreamToken{Done: true, Error: Classify(err, 0, err.Error(), "claude")}
        }
    }()
    return out, nil
}

func (p *ClaudeProvider) CountTokens(messages []Message) int {
    total := 0
    for _, m := range messages {
        total += len(m.Content) / 4
        total += 4
    }
    return total
}
