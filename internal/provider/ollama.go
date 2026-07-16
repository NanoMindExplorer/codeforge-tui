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

// OllamaProvider talks to a local Ollama instance (offline mode).
type OllamaProvider struct {
	endpoint string
	model    string
	client   *http.Client
}

func NewOllamaProvider(defaultModel string) *OllamaProvider {
	if defaultModel == "" {
		defaultModel = os.Getenv("OLLAMA_MODEL")
	}
	if defaultModel == "" {
		defaultModel = "llama3.2"
	}
	endpoint := os.Getenv("OLLAMA_HOST")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	endpoint = strings.TrimRight(endpoint, "/")
	return &OllamaProvider{
		endpoint: endpoint,
		model:    defaultModel,
		client:   &http.Client{Timeout: 300 * time.Second},
	}
}

func (p *OllamaProvider) Name() string  { return "ollama" }
func (p *OllamaProvider) Model() string { return p.model }

func (p *OllamaProvider) SetModel(id string) error {
	if id == "" {
		return fmt.Errorf("model id required")
	}
	p.model = id
	return nil
}

func (p *OllamaProvider) Models() []ModelInfo {
	return []ModelInfo{
		{ID: "llama3.2", Name: "Llama 3.2 (local)", ContextWindow: 128000, InputCost: 0, OutputCost: 0},
		{ID: "qwen2.5-coder", Name: "Qwen2.5 Coder (local)", ContextWindow: 32768, InputCost: 0, OutputCost: 0},
		{ID: "codellama", Name: "Code Llama (local)", ContextWindow: 16384, InputCost: 0, OutputCost: 0},
		{ID: "mistral", Name: "Mistral (local)", ContextWindow: 32768, InputCost: 0, OutputCost: 0},
	}
}

func (p *OllamaProvider) ValidateConfig() error {
	// Soft check — try /api/tags
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", p.endpoint+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama not reachable at %s: %w", p.endpoint, err)
	}
	resp.Body.Close()
	return nil
}

type ollamaMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaReq struct {
	Model    string      `json:"model"`
	Messages []ollamaMsg `json:"messages"`
	Stream   bool        `json:"stream"`
	Options  map[string]any `json:"options,omitempty"`
}

type ollamaResp struct {
	Message ollamaMsg `json:"message"`
	Done    bool      `json:"done"`
	Error   string    `json:"error,omitempty"`
}

func toOllamaMessages(msgs []Message, system string) []ollamaMsg {
	out := make([]ollamaMsg, 0, len(msgs)+1)
	if system != "" {
		out = append(out, ollamaMsg{Role: "system", Content: system})
	}
	for _, m := range msgs {
		role := m.Role
		if role == RoleTool {
			role = "user"
			m.Content = "[tool result]\n" + m.Content
		}
		if role == RoleAssistant {
			role = "assistant"
		}
		if role == RoleUser {
			role = "user"
		}
		out = append(out, ollamaMsg{Role: role, Content: m.Content})
	}
	return out
}

func (p *OllamaProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	// Note: basic tool calling not fully supported on all Ollama models —
	// we still send conversation; agent may get text-only answers offline.
	body, _ := json.Marshal(ollamaReq{
		Model:    model,
		Messages: toOllamaMessages(req.Messages, req.System),
		Stream:   false,
	})
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(raw))
	}
	var or ollamaResp
	if err := json.Unmarshal(raw, &or); err != nil {
		return nil, err
	}
	if or.Error != "" {
		return nil, fmt.Errorf("ollama: %s", or.Error)
	}
	return &CompletionResponse{
		Content:    or.Message.Content,
		StopReason: "stop",
	}, nil
}

func (p *OllamaProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamToken, error) {
	model := req.Model
	if model == "" {
		model = p.model
	}
	body, _ := json.Marshal(ollamaReq{
		Model:    model,
		Messages: toOllamaMessages(req.Messages, req.System),
		Stream:   true,
	})
	out := make(chan StreamToken, 100)
	go func() {
		defer close(out)
		httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/api/chat", bytes.NewReader(body))
		if err != nil {
			out <- StreamToken{Done: true, Error: err}
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		resp, err := p.client.Do(httpReq)
		if err != nil {
			out <- StreamToken{Done: true, Error: err}
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			b, _ := io.ReadAll(resp.Body)
			out <- StreamToken{Done: true, Error: fmt.Errorf("ollama %d: %s", resp.StatusCode, string(b))}
			return
		}
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var chunk ollamaResp
			if err := json.Unmarshal(sc.Bytes(), &chunk); err != nil {
				continue
			}
			if chunk.Message.Content != "" {
				out <- StreamToken{Text: chunk.Message.Content}
			}
			if chunk.Done {
				out <- StreamToken{Done: true}
				return
			}
		}
		out <- StreamToken{Done: true}
	}()
	return out, nil
}

func (p *OllamaProvider) CountTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)/4 + 4
	}
	return total
}
