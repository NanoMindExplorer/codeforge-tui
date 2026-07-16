package provider

import (
    "context"
    "errors"
    "fmt"
)

// Role constants for provider.Message.Role.
const (
    RoleUser      = "user"
    RoleAssistant = "assistant"
    RoleTool      = "tool"
)

type Message struct {
    Role    string `json:"role"` // "user" | "assistant" | "tool"
    Content string `json:"content"`

    // ToolCalls is set on assistant messages that invoke one or more tools.
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`

    // The following three fields are set on role="tool" messages, which
    // carry the result of executing a single tool call back to the model.
    ToolCallID string `json:"tool_call_id,omitempty"` // references ToolCall.ID
    ToolName   string `json:"tool_name,omitempty"`
    IsError    bool   `json:"is_error,omitempty"`
}

type ToolCall struct {
    ID    string `json:"id"`
    Name  string `json:"name"`
    Input string `json:"input"` // raw JSON arguments object
}

type ToolDefinition struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    InputSchema any    `json:"input_schema"`
}

type CompletionRequest struct {
    Messages    []Message        `json:"messages"`
    Model       string           `json:"model"`
    MaxTokens   int              `json:"max_tokens,omitempty"`
    Temperature float64          `json:"temperature,omitempty"`
    System      string           `json:"system,omitempty"`
    Tools       []ToolDefinition `json:"tools,omitempty"`
}

type CompletionResponse struct {
    Content      string     `json:"content"`
    ToolCalls    []ToolCall `json:"tool_calls,omitempty"`
    InputTokens  int        `json:"input_tokens"`
    OutputTokens int        `json:"output_tokens"`
    StopReason   string     `json:"stop_reason"`
}

type StreamToken struct {
    Text         string    `json:"text,omitempty"`
    ToolCall     *ToolCall `json:"tool_call,omitempty"`
    Done         bool      `json:"done"`
    Error        error     `json:"-"`
    InputTokens  int       `json:"input_tokens,omitempty"`
    OutputTokens int       `json:"output_tokens,omitempty"`
}

type Provider interface {
    Name() string
    Models() []ModelInfo
    // Model returns the currently selected model ID.
    Model() string
    // SetModel switches the active model. Returns error if ID is unknown.
    SetModel(id string) error
    Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
    Stream(ctx context.Context, req CompletionRequest) (<-chan StreamToken, error)
    CountTokens(messages []Message) int
    ValidateConfig() error
}

// CostForModel returns USD cost for token counts using ModelInfo pricing.
func CostForModel(p Provider, modelID string, in, out int) float64 {
    if p == nil {
        return 0
    }
    if modelID == "" {
        modelID = p.Model()
    }
    var chosen *ModelInfo
    for i := range p.Models() {
        m := p.Models()[i]
        if m.ID == modelID {
            chosen = &m
            break
        }
    }
    if chosen == nil {
        models := p.Models()
        if len(models) == 0 {
            return 0
        }
        chosen = &models[0]
    }
    return float64(in)*chosen.InputCost/1_000_000 + float64(out)*chosen.OutputCost/1_000_000
}

type ModelInfo struct {
    ID            string  `json:"id"`
    Name          string  `json:"name"`
    ContextWindow int     `json:"context_window"`
    InputCost     float64 `json:"input_cost_per_1m"`
    OutputCost    float64 `json:"output_cost_per_1m"`
}

type Registry struct {
    providers map[string]Provider
    current   string
}

func NewRegistry() *Registry {
    return &Registry{providers: make(map[string]Provider)}
}

func (r *Registry) Register(p Provider) error {
    if p == nil {
        return errors.New("provider is nil")
    }
    r.providers[p.Name()] = p
    if r.current == "" {
        r.current = p.Name()
    }
    return nil
}

func (r *Registry) Get(name string) (Provider, error) {
    p, ok := r.providers[name]
    if !ok {
        return nil, fmt.Errorf("provider %q not registered", name)
    }
    return p, nil
}

func (r *Registry) Current() (Provider, error) {
    if r.current == "" {
        return nil, errors.New("no provider registered")
    }
    return r.Get(r.current)
}

func (r *Registry) Switch(name string) error {
    if _, ok := r.providers[name]; !ok {
        return fmt.Errorf("provider %q not registered", name)
    }
    r.current = name
    return nil
}

func (r *Registry) List() []string {
    names := make([]string, 0, len(r.providers))
    for name := range r.providers {
        names = append(names, name)
    }
    return names
}

func (r *Registry) CurrentName() string {
    return r.current
}
