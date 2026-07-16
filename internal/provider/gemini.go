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

// GeminiProvider mengimplementasi Provider interface untuk Google Gemini API.
type GeminiProvider struct {
    apiKey string
    model  string
    client *http.Client
}

type geminiContent struct {
    Role  string       `json:"role"`
    Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
    Text             string                   `json:"text,omitempty"`
    FunctionCall     *geminiFunctionCall      `json:"functionCall,omitempty"`
    FunctionResponse *geminiFunctionResponse  `json:"functionResponse,omitempty"`
}

type geminiFunctionCall struct {
    Name string         `json:"name"`
    Args map[string]any `json:"args"`
}

type geminiFunctionResponse struct {
    Name     string         `json:"name"`
    Response map[string]any `json:"response"`
}

type geminiTool struct {
    FunctionDeclarations []geminiFunctionDeclaration `json:"functionDeclarations"`
}

type geminiFunctionDeclaration struct {
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    Parameters  any    `json:"parameters,omitempty"`
}

type geminiRequest struct {
    Contents          []geminiContent  `json:"contents"`
    SystemInstruction *geminiContent   `json:"systemInstruction,omitempty"`
    GenerationConfig  *geminiGenConfig `json:"generationConfig,omitempty"`
    Tools             []geminiTool     `json:"tools,omitempty"`
}

type geminiGenConfig struct {
    MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
    Temperature     float64 `json:"temperature,omitempty"`
}

type geminiResponse struct {
    Candidates []struct {
        Content      geminiContent `json:"content"`
        FinishReason string        `json:"finishReason"`
    } `json:"candidates"`
    UsageMetadata struct {
        PromptTokenCount     int `json:"promptTokenCount"`
        CandidatesTokenCount int `json:"candidatesTokenCount"`
        TotalTokenCount      int `json:"totalTokenCount"`
    } `json:"usageMetadata"`
    Error *geminiError `json:"error,omitempty"`
}

type geminiError struct {
    Code    int    `json:"code"`
    Message string `json:"message"`
    Status  string `json:"status"`
}

func NewGeminiProvider(apiKey, defaultModel string) *GeminiProvider {
    if apiKey == "" {
        apiKey = os.Getenv("GEMINI_API_KEY")
    }
    if defaultModel == "" {
        defaultModel = "gemini-2.5-flash"
    }
    return &GeminiProvider{
        apiKey: apiKey,
        model:  defaultModel,
        client: &http.Client{Timeout: 180 * time.Second},
    }
}

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) Model() string { return p.model }

func (p *GeminiProvider) SetModel(id string) error {
    if id == "" {
        return fmt.Errorf("model id required")
    }
    p.model = id
    return nil
}

func (p *GeminiProvider) Models() []ModelInfo {
    // Official list pricing (USD / 1M tokens). Flash free-tier is $0 under quota;
    // Pro is paid. Using published Google AI Studio rates as of 2025.
    return []ModelInfo{
        {ID: "gemini-2.5-flash", Name: "Gemini 2.5 Flash", ContextWindow: 1048576, InputCost: 0.15, OutputCost: 0.60},
        {ID: "gemini-2.0-flash", Name: "Gemini 2.0 Flash", ContextWindow: 1048576, InputCost: 0.10, OutputCost: 0.40},
        {ID: "gemini-2.5-pro", Name: "Gemini 2.5 Pro", ContextWindow: 1048576, InputCost: 1.25, OutputCost: 10.0},
        {ID: "gemini-2.0-flash-lite", Name: "Gemini 2.0 Flash Lite", ContextWindow: 1048576, InputCost: 0.075, OutputCost: 0.30},
    }
}

func (p *GeminiProvider) ValidateConfig() error {
    if p.apiKey == "" {
        return fmt.Errorf("GEMINI_API_KEY not set. Get free key at https://aistudio.google.com/apikey")
    }
    return nil
}

// toGeminiContents converts the provider-agnostic conversation into Gemini's
// content format, including assistant tool calls (role "model" with
// functionCall parts) and tool results (role "function" with
// functionResponse parts).
func toGeminiContents(msgs []Message) []geminiContent {
    out := make([]geminiContent, 0, len(msgs))
    for _, m := range msgs {
        switch m.Role {
        case RoleTool:
            respObj := map[string]any{"result": m.Content}
            if m.IsError {
                respObj = map[string]any{"error": m.Content}
            }
            out = append(out, geminiContent{
                Role: "function",
                Parts: []geminiPart{{
                    FunctionResponse: &geminiFunctionResponse{
                        Name:     m.ToolName,
                        Response: respObj,
                    },
                }},
            })
        case RoleAssistant:
            var parts []geminiPart
            if m.Content != "" {
                parts = append(parts, geminiPart{Text: m.Content})
            }
            for _, tc := range m.ToolCalls {
                var args map[string]any
                if err := json.Unmarshal([]byte(tc.Input), &args); err != nil {
                    args = map[string]any{}
                }
                parts = append(parts, geminiPart{
                    FunctionCall: &geminiFunctionCall{Name: tc.Name, Args: args},
                })
            }
            if len(parts) == 0 {
                parts = []geminiPart{{Text: ""}}
            }
            out = append(out, geminiContent{Role: "model", Parts: parts})
        default:
            out = append(out, geminiContent{
                Role:  "user",
                Parts: []geminiPart{{Text: m.Content}},
            })
        }
    }
    return out
}

func toGeminiTools(defs []ToolDefinition) []geminiTool {
    if len(defs) == 0 {
        return nil
    }
    fns := make([]geminiFunctionDeclaration, 0, len(defs))
    for _, d := range defs {
        fns = append(fns, geminiFunctionDeclaration{
            Name:        d.Name,
            Description: d.Description,
            Parameters:  d.InputSchema,
        })
    }
    return []geminiTool{{FunctionDeclarations: fns}}
}

func (p *GeminiProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
    if err := p.ValidateConfig(); err != nil {
        return nil, err
    }
    model := req.Model
    if model == "" {
        model = p.model
    }
    maxTokens := req.MaxTokens
    if maxTokens == 0 {
        maxTokens = 8192
    }

    gReq := geminiRequest{
        Contents: toGeminiContents(req.Messages),
        GenerationConfig: &geminiGenConfig{
            MaxOutputTokens: maxTokens,
            Temperature:     req.Temperature,
        },
        Tools: toGeminiTools(req.Tools),
    }
    if req.System != "" {
        gReq.SystemInstruction = &geminiContent{
            Parts: []geminiPart{{Text: req.System}},
        }
    }

    body, _ := json.Marshal(gReq)
    url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, p.apiKey)

    httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
    if err != nil {
        return nil, err
    }
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := p.client.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("gemini http: %w", err)
    }
    defer resp.Body.Close()

    respBody, _ := io.ReadAll(resp.Body)
    if resp.StatusCode != 200 {
        return nil, fmt.Errorf("gemini error (status %d): %s", resp.StatusCode, string(respBody))
    }

    var gResp geminiResponse
    if err := json.Unmarshal(respBody, &gResp); err != nil {
        return nil, fmt.Errorf("gemini parse: %w", err)
    }
    if gResp.Error != nil {
        return nil, fmt.Errorf("gemini: %s", gResp.Error.Message)
    }

    result := &CompletionResponse{
        InputTokens:  gResp.UsageMetadata.PromptTokenCount,
        OutputTokens: gResp.UsageMetadata.CandidatesTokenCount,
    }
    for _, cand := range gResp.Candidates {
        for i, part := range cand.Content.Parts {
            if part.Text != "" {
                result.Content += part.Text
            }
            if part.FunctionCall != nil {
                argsJSON, _ := json.Marshal(part.FunctionCall.Args)
                result.ToolCalls = append(result.ToolCalls, ToolCall{
                    ID:    fmt.Sprintf("call_%d_%s", i, part.FunctionCall.Name),
                    Name:  part.FunctionCall.Name,
                    Input: string(argsJSON),
                })
            }
        }
        if cand.FinishReason != "" {
            result.StopReason = cand.FinishReason
        }
    }
    return result, nil
}

// Stream provides plain text streaming and does not support tool calling.
// The agent loop uses Complete for turns where the model may call tools;
// Stream remains available for simple, tool-free chat.
func (p *GeminiProvider) Stream(ctx context.Context, req CompletionRequest) (<-chan StreamToken, error) {
    if err := p.ValidateConfig(); err != nil {
        return nil, err
    }
    model := req.Model
    if model == "" {
        model = p.model
    }
    maxTokens := req.MaxTokens
    if maxTokens == 0 {
        maxTokens = 8192
    }

    gReq := geminiRequest{
        Contents: toGeminiContents(req.Messages),
        GenerationConfig: &geminiGenConfig{
            MaxOutputTokens: maxTokens,
            Temperature:     req.Temperature,
        },
    }
    if req.System != "" {
        gReq.SystemInstruction = &geminiContent{
            Parts: []geminiPart{{Text: req.System}},
        }
    }

    body, _ := json.Marshal(gReq)
    url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", model, p.apiKey)

    out := make(chan StreamToken, 200)

    go func() {
        defer close(out)

        httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
        if err != nil {
            out <- StreamToken{Done: true, Error: err}
            return
        }
        httpReq.Header.Set("Content-Type", "application/json")

        resp, err := p.client.Do(httpReq)
        if err != nil {
            // Ignore context canceled - normal saat user kirim message baru
            errMsg := err.Error()
            if !strings.Contains(errMsg, "context canceled") {
                out <- StreamToken{Done: true, Error: fmt.Errorf("gemini http: %w", err)}
            } else {
                out <- StreamToken{Done: true}
            }
            return
        }
        defer resp.Body.Close()

        if resp.StatusCode != 200 {
            respBody, _ := io.ReadAll(resp.Body)
            out <- StreamToken{Done: true, Error: fmt.Errorf("gemini error %d: %s", resp.StatusCode, string(respBody))}
            return
        }

        scanner := bufio.NewScanner(resp.Body)
        scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
        var inputTok, outputTok int
        doneSent := false

        for scanner.Scan() {
            line := scanner.Text()
            if !strings.HasPrefix(line, "data: ") {
                continue
            }
            data := strings.TrimPrefix(line, "data: ")
            if data == "" || data == "[DONE]" {
                continue
            }
            var chunk geminiResponse
            if err := json.Unmarshal([]byte(data), &chunk); err != nil {
                continue
            }
            if chunk.Error != nil {
                if !doneSent {
                    doneSent = true
                    out <- StreamToken{Done: true, Error: fmt.Errorf("gemini: %s", chunk.Error.Message)}
                }
                return
            }
            // Stream text chunks
            for _, cand := range chunk.Candidates {
                for _, part := range cand.Content.Parts {
                    if part.Text != "" {
                        out <- StreamToken{Text: part.Text}
                    }
                }
            }
            // Update token counts
            if chunk.UsageMetadata.TotalTokenCount > 0 {
                inputTok = chunk.UsageMetadata.PromptTokenCount
                outputTok = chunk.UsageMetadata.CandidatesTokenCount
            }
        }

        // Scanner finished - send Done with token counts
        if !doneSent {
            out <- StreamToken{
                Done:         true,
                InputTokens:  inputTok,
                OutputTokens: outputTok,
            }
        }
    }()

    return out, nil
}

func (p *GeminiProvider) CountTokens(messages []Message) int {
    total := 0
    for _, m := range messages {
        total += len(m.Content) / 4
        total += 4
    }
    return total
}
