package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAIProvider implements OpenAI-compatible chat completions
// (OpenAI, Azure OpenAI, Groq, Together, etc. via endpoint override).
type OpenAIProvider struct {
	apiKey   string
	model    string
	endpoint string // default https://api.openai.com/v1
	client   *http.Client
	name     string
}

func NewOpenAIProvider(apiKey, defaultModel string) *OpenAIProvider {
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if defaultModel == "" {
		defaultModel = "gpt-4o-mini"
	}
	endpoint := os.Getenv("OPENAI_BASE_URL")
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	return &OpenAIProvider{
		apiKey:   apiKey,
		model:    defaultModel,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 180 * time.Second},
		name:     "openai",
	}
}

func (p *OpenAIProvider) Name() string  { return p.name }
func (p *OpenAIProvider) Model() string { return p.model }

func (p *OpenAIProvider) SetModel(id string) error {
	if id == "" {
		return fmt.Errorf("model id required")
	}
	p.model = id
	return nil
}

func (p *OpenAIProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: "gpt-4o", Name: "GPT-4o", ContextWindow: 128000, InputCost: 2.5, OutputCost: 10.0},
		{ID: "gpt-4o-mini", Name: "GPT-4o Mini", ContextWindow: 128000, InputCost: 0.15, OutputCost: 0.60},
		{ID: "o3-mini", Name: "o3-mini", ContextWindow: 200000, InputCost: 1.1, OutputCost: 4.4},
	}
}

func (p *OpenAIProvider) ValidateConfig() error {
	if p.apiKey == "" {
		return fmt.Errorf("OPENAI_API_KEY not set")
	}
	return nil
}

type oaiMsg struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []oaiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
	Name       string        `json:"name,omitempty"`
}

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiTool struct {
	Type     string `json:"type"`
	Function struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Parameters  any    `json:"parameters"`
	} `json:"function"`
}

type oaiReq struct {
	Model       string    `json:"model"`
	Messages    []oaiMsg  `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Tools       []oaiTool `json:"tools,omitempty"`
}

type oaiResp struct {
	Choices []struct {
		Message      oaiMsg `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func toOpenAIMessages(msgs []Message, system string) []oaiMsg {
	out := make([]oaiMsg, 0, len(msgs)+1)
	if system != "" {
		out = append(out, oaiMsg{Role: "system", Content: system})
	}
	for _, m := range msgs {
		switch m.Role {
		case RoleTool:
			out = append(out, oaiMsg{
				Role:       "tool",
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
				Name:       m.ToolName,
			})
		case RoleAssistant:
			om := oaiMsg{Role: "assistant", Content: m.Content}
			for _, tc := range m.ToolCalls {
				var c oaiToolCall
				c.ID = tc.ID
				c.Type = "function"
				c.Function.Name = tc.Name
				c.Function.Arguments = tc.Input
				om.ToolCalls = append(om.ToolCalls, c)
			}
			out = append(out, om)
		default:
			out = append(out, oaiMsg{Role: "user", Content: m.Content})
		}
	}
	return out
}

func toOpenAITools(defs []ToolDefinition) []oaiTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]oaiTool, 0, len(defs))
	for _, d := range defs {
		var t oaiTool
		t.Type = "function"
		t.Function.Name = d.Name
		t.Function.Description = d.Description
		t.Function.Parameters = d.InputSchema
		out = append(out, t)
	}
	return out
}

func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
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
	body, _ := json.Marshal(oaiReq{
		Model:       model,
		Messages:    toOpenAIMessages(req.Messages, req.System),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Tools:       toOpenAITools(req.Tools),
	})
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("openai error %d: %s", resp.StatusCode, string(raw))
	}
	var oai oaiResp
	if err := json.Unmarshal(raw, &oai); err != nil {
		return nil, err
	}
	if oai.Error != nil {
		return nil, fmt.Errorf("openai: %s", oai.Error.Message)
	}
	result := &CompletionResponse{
		InputTokens:  oai.Usage.PromptTokens,
		OutputTokens: oai.Usage.CompletionTokens,
	}
	if len(oai.Choices) > 0 {
		msg := oai.Choices[0].Message
		result.Content = msg.Content
		result.StopReason = oai.Choices[0].FinishReason
		for _, tc := range msg.ToolCalls {
			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:    tc.ID,
				Name:  tc.Function.Name,
				Input: tc.Function.Arguments,
			})
		}
	}
	return result, nil
}

func (p *OpenAIProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamToken, error) {
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
	body, _ := json.Marshal(oaiReq{
		Model:       model,
		Messages:    toOpenAIMessages(req.Messages, req.System),
		MaxTokens:   maxTokens,
		Temperature: req.Temperature,
		Stream:      true,
	})
	out := make(chan StreamToken, 100)
	go func() {
		defer close(out)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			out <- StreamToken{Done: true, Error: err}
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

		resp, err := p.client.Do(httpReq)
		if err != nil {
			out <- StreamToken{Done: true, Error: err}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			out <- StreamToken{Done: true, Error: fmt.Errorf("openai %d: %s", resp.StatusCode, string(b))}
			return
		}
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				out <- StreamToken{Done: true}
				return
			}
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				continue
			}
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				out <- StreamToken{Text: chunk.Choices[0].Delta.Content}
			}
		}
		out <- StreamToken{Done: true}
	}()
	return out, nil
}

func (p *OpenAIProvider) CountTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}
