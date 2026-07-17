package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrorCode classifies provider failures for UX and automation.
type ErrorCode string

const (
	ErrAuth        ErrorCode = "auth"
	ErrRateLimit   ErrorCode = "rate_limit"
	ErrQuota       ErrorCode = "quota"
	ErrModel       ErrorCode = "model"
	ErrContext     ErrorCode = "context"
	ErrNetwork     ErrorCode = "network"
	ErrTimeout     ErrorCode = "timeout"
	ErrUnsupported ErrorCode = "unsupported"
	ErrUnknown     ErrorCode = "unknown"
)

// ProviderError is a structured, user-facing provider failure.
// Message/Hint are always safe for the TUI (no stack traces, no raw JSON dumps).
type ProviderError struct {
	Code       ErrorCode
	Message    string        // short human message
	Hint       string        // next step
	Retry      bool          // safe to retry
	RetryAfter time.Duration // optional
	Status     int           // HTTP status if any
	Provider   string        // grok|openai|gemini|claude|ollama
	Raw        string        // truncated redacted body for logs only — never shown in UserMessage
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "provider error"
	}
	msg := e.Message
	if e.Hint != "" {
		msg += " — " + e.Hint
	}
	return msg
}

func (e *ProviderError) Unwrap() error { return nil }

// Short is a single-line toast-friendly string (no multi-line, no code).
func (e *ProviderError) Short() string {
	if e == nil {
		return "Provider error"
	}
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s (retry ~%s)", e.Message, e.RetryAfter.Round(time.Second))
	}
	return e.Message
}

// UserMessage is multi-line text safe for TUI system blocks.
// Never includes Raw, stacks, or full API JSON.
func (e *ProviderError) UserMessage() string {
	if e == nil {
		return "⚠ Unknown provider error"
	}
	var b strings.Builder
	b.WriteString(iconFor(e.Code))
	b.WriteString(" ")
	b.WriteString(e.Message)
	if e.Provider != "" {
		b.WriteString(fmt.Sprintf("  [%s]", e.Provider))
	}
	if e.Hint != "" {
		b.WriteString("\n  → ")
		b.WriteString(e.Hint)
	}
	if e.Retry && e.RetryAfter > 0 {
		b.WriteString(fmt.Sprintf("\n  ↻ retry after ~%s", e.RetryAfter.Round(time.Second)))
	} else if e.Retry {
		b.WriteString("\n  ↻ safe to retry")
	}
	if e.Code != "" && e.Code != ErrUnknown {
		b.WriteString(fmt.Sprintf("\n  code: %s", e.Code))
	}
	if e.Status > 0 && e.Code == ErrUnknown {
		b.WriteString(fmt.Sprintf("\n  http: %d", e.Status))
	}
	return b.String()
}

func iconFor(c ErrorCode) string {
	switch c {
	case ErrAuth:
		return "🔑"
	case ErrRateLimit:
		return "⏳"
	case ErrQuota:
		return "💳"
	case ErrModel:
		return "🧩"
	case ErrContext:
		return "📎"
	case ErrNetwork:
		return "🌐"
	case ErrTimeout:
		return "⏱"
	case ErrUnsupported:
		return "🧠"
	default:
		return "⚠"
	}
}

// AsProviderError extracts *ProviderError from err chain.
func AsProviderError(err error) (*ProviderError, bool) {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe, true
	}
	if err == nil {
		return nil, false
	}
	return Classify(err, 0, "", ""), true
}

// userMessenger is implemented by ProviderError and agent.LoopError.
type userMessenger interface {
	UserMessage() string
}

// FormatUserError turns any error into a friendly multi-line string for the TUI.
func FormatUserError(err error) string {
	if err == nil {
		return ""
	}
	if um, ok := err.(userMessenger); ok {
		return um.UserMessage()
	}
	if pe, ok := err.(*ProviderError); ok {
		return pe.UserMessage()
	}
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.UserMessage()
	}
	return Classify(err, 0, err.Error(), "").UserMessage()
}

// FormatUserErrorShort is for toasts (one line).
func FormatUserErrorShort(err error) string {
	if err == nil {
		return ""
	}
	pe, _ := AsProviderError(err)
	if pe != nil {
		return pe.Short()
	}
	s := err.Error()
	if len(s) > 80 {
		return s[:77] + "…"
	}
	return s
}

// Classify maps transport/API failures to ProviderError.
func Classify(err error, status int, body, provider string) *ProviderError {
	// Prefer structured message from JSON body for heuristics, keep raw redacted for logs.
	apiMsg := extractAPIMessage(body)
	raw := redactSecrets(truncateRaw(body, 400))
	combined := body
	if apiMsg != "" {
		combined = apiMsg + " " + body
	}
	low := strings.ToLower(combined + " " + errString(err))

	pe := &ProviderError{
		Code:     ErrUnknown,
		Status:   status,
		Provider: provider,
		Raw:      raw,
		Message:  "Provider request failed",
		Hint:     "Check network and API key, then retry — or /doctor",
		Retry:    true,
	}

	// Network / timeout
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			pe.Code = ErrTimeout
			pe.Message = "Request timed out"
			pe.Hint = "Retry, or use a faster model (/model)"
			pe.Retry = true
			logProviderError(pe)
			return pe
		}
		if errors.Is(err, context.Canceled) || strings.Contains(low, "context canceled") {
			pe.Code = ErrTimeout
			pe.Message = "Request canceled"
			pe.Hint = "You stopped the turn, or a new message interrupted it"
			pe.Retry = false
			return pe // no log spam for cancel
		}
		if errors.Is(err, context.DeadlineExceeded) || strings.Contains(low, "deadline exceeded") {
			pe.Code = ErrTimeout
			pe.Message = "Request timed out"
			pe.Hint = "Retry or use a faster model (/model)"
			pe.Retry = true
			logProviderError(pe)
			return pe
		}
		if strings.Contains(low, "connection refused") || strings.Contains(low, "no such host") ||
			strings.Contains(low, "network is unreachable") || strings.Contains(low, "tls") ||
			strings.Contains(low, "x509") || strings.Contains(low, "eof") {
			pe.Code = ErrNetwork
			pe.Message = "Cannot reach the API"
			pe.Hint = networkHint(provider)
			pe.Retry = true
			logProviderError(pe)
			return pe
		}
	}

	// HTTP status
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		pe.Code = ErrAuth
		pe.Message = "API key rejected or missing"
		pe.Hint = authHint(provider)
		pe.Retry = false
		logProviderError(pe)
		return pe
	case http.StatusTooManyRequests:
		pe.Code = ErrRateLimit
		pe.Message = "Rate limited by the provider"
		pe.Hint = "Wait a moment, lower concurrency, or switch model (/model)"
		pe.Retry = true
		if d := parseRetryAfter(body); d > 0 {
			pe.RetryAfter = d
			pe.Hint = fmt.Sprintf("Wait ~%s, or switch model (/model)", d.Round(time.Second))
		}
		logProviderError(pe)
		return pe
	case http.StatusPaymentRequired:
		pe.Code = ErrQuota
		pe.Message = "Quota or billing limit reached"
		pe.Hint = "Check provider console billing / plan"
		pe.Retry = false
		logProviderError(pe)
		return pe
	case http.StatusBadRequest:
		classifyBadRequest(pe, low, provider)
		logProviderError(pe)
		return pe
	case http.StatusNotFound:
		if strings.Contains(low, "model") || apiMsg != "" {
			pe.Code = ErrModel
			pe.Message = "Model not found"
			pe.Hint = "Run /model to pick a valid id"
			pe.Retry = false
			logProviderError(pe)
			return pe
		}
	case http.StatusRequestEntityTooLarge:
		pe.Code = ErrContext
		pe.Message = "Request too large"
		pe.Hint = "Use /compact or shorten the conversation"
		pe.Retry = false
		logProviderError(pe)
		return pe
	case 529, 503, 502:
		pe.Code = ErrNetwork
		pe.Message = "Provider temporarily unavailable"
		pe.Hint = "Retry in a few seconds"
		pe.Retry = true
		logProviderError(pe)
		return pe
	}

	// Body heuristics (when status missing or 200-wrapped errors)
	switch {
	case containsAny(low, "invalid api key", "incorrect api key", "authentication", "unauthorized",
		"api key not set", "api_key not set", "not set", "permission denied", "invalid_api_key"):
		pe.Code = ErrAuth
		pe.Message = "API key rejected or missing"
		pe.Hint = authHint(provider)
		pe.Retry = false
	case containsAny(low, "rate limit", "rate_limit", "too many requests", "resource_exhausted", "429"):
		if containsAny(low, "billing", "insufficient_quota", "credit", "quota exceeded", "payment") {
			pe.Code = ErrQuota
			pe.Message = "Quota or billing limit reached"
			pe.Hint = "Check provider console billing / plan"
			pe.Retry = false
		} else {
			pe.Code = ErrRateLimit
			pe.Message = "Rate limited by the provider"
			pe.Hint = "Wait a moment or switch model (/model)"
			pe.Retry = true
			if d := parseRetryAfter(body); d > 0 {
				pe.RetryAfter = d
			}
		}
	case containsAny(low, "insufficient_quota", "quota exceeded", "billing", "payment required"):
		pe.Code = ErrQuota
		pe.Message = "Quota or billing limit reached"
		pe.Hint = "Check provider console billing / plan"
		pe.Retry = false
	case containsAny(low, "context_length", "maximum context", "too many tokens", "token limit",
		"context window", "prompt is too long", "max_tokens"):
		pe.Code = ErrContext
		pe.Message = "Context too long for this model"
		pe.Hint = "Run /compact or start /new session"
		pe.Retry = false
	case containsAny(low, "include_reasoning", "reasoning_effort", "budget_tokens", "thinking_type",
		"thinking blocks", "extended thinking"):
		pe.Code = ErrUnsupported
		pe.Message = "Reasoning/thinking not supported for this request"
		pe.Hint = "CodeForge will retry without thinking, or set CODEFORGE_REASONING=off"
		pe.Retry = true
	case containsAny(low, "model_not_found", "does not exist", "unknown model", "invalid model", "model not found"):
		pe.Code = ErrModel
		pe.Message = "Model not available"
		pe.Hint = "Check /model list or provider catalog"
		pe.Retry = false
	case containsAny(low, "timeout", "deadline exceeded", "timed out"):
		pe.Code = ErrTimeout
		pe.Message = "Request timed out"
		pe.Hint = "Retry or use a faster model"
		pe.Retry = true
	default:
		// Never surface stacks / raw JSON as the main message
		if status > 0 {
			pe.Message = fmt.Sprintf("Provider error (HTTP %d)", status)
			if apiMsg != "" {
				pe.Hint = sanitizeHint(apiMsg)
			} else {
				pe.Hint = "Retry, run /doctor, or switch provider (/provider)"
			}
		} else if err != nil {
			pe.Message = "Provider request failed"
			pe.Hint = sanitizeHint(err.Error())
			if pe.Hint == "" || looksUnsafeForUser(err.Error()) {
				pe.Hint = "Check network and API key, then retry — or /doctor"
			}
		}
	}
	logProviderError(pe)
	return pe
}

func classifyBadRequest(pe *ProviderError, low, provider string) *ProviderError {
	switch {
	case containsAny(low, "include_reasoning", "reasoning_effort", "budget_tokens", "thinking", "reasoning"):
		pe.Code = ErrUnsupported
		pe.Message = "This model rejected reasoning/thinking parameters"
		pe.Hint = "CodeForge retries without thinking automatically, or set CODEFORGE_REASONING=off"
		pe.Retry = true
	case containsAny(low, "context", "token", "too long", "maximum", "prompt is too long"):
		pe.Code = ErrContext
		pe.Message = "Request rejected (likely context/token limit)"
		pe.Hint = "Use /compact or shorten input"
		pe.Retry = false
	case containsAny(low, "model"):
		pe.Code = ErrModel
		pe.Message = "Invalid model or request for this model"
		pe.Hint = "Try /model with a supported id"
		pe.Retry = false
	default:
		pe.Code = ErrUnknown
		pe.Message = "Bad request to provider API"
		pe.Hint = "Check model id and request options (/model, /doctor)"
		pe.Retry = false
	}
	_ = provider
	return pe
}

func authHint(provider string) string {
	switch strings.ToLower(provider) {
	case "grok", "xai":
		return "Set XAI_API_KEY or GROK_API_KEY, then /provider grok · /setup"
	case "gemini":
		return "Set GEMINI_API_KEY (https://aistudio.google.com/apikey) · /setup gemini"
	case "claude", "anthropic":
		return "Set ANTHROPIC_API_KEY · /setup claude"
	case "openai":
		return "Set OPENAI_API_KEY (and OPENAI_BASE_URL if using a proxy) · /setup openai"
	case "ollama":
		return "Start Ollama locally (ollama serve) and pull a model · /setup ollama"
	default:
		return "Run /setup or set the provider API key, then /provider"
	}
}

func networkHint(provider string) string {
	switch strings.ToLower(provider) {
	case "grok", "xai":
		return "Check network or XAI_BASE_URL"
	case "openai":
		return "Check network or OPENAI_BASE_URL"
	case "ollama":
		return "Is ollama serve running? Check OLLAMA_HOST"
	default:
		return "Check network, proxy, or base URL"
	}
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func truncateRaw(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func parseRetryAfter(body string) time.Duration {
	re := regexp.MustCompile(`(?i)"retry[_-]after"\s*:\s*(\d+)`)
	if m := re.FindStringSubmatch(body); len(m) == 2 {
		if sec, err := strconv.Atoi(m[1]); err == nil && sec > 0 {
			return time.Duration(sec) * time.Second
		}
	}
	// header-style seconds only digits in body
	re2 := regexp.MustCompile(`(?i)retry.after[=:\s]+(\d+)`)
	if m := re2.FindStringSubmatch(body); len(m) == 2 {
		if sec, err := strconv.Atoi(m[1]); err == nil && sec > 0 {
			return time.Duration(sec) * time.Second
		}
	}
	return 0
}

// extractAPIMessage pulls human message fields from common provider JSON shapes.
func extractAPIMessage(body string) string {
	body = strings.TrimSpace(body)
	if body == "" || !strings.HasPrefix(body, "{") {
		return ""
	}
	var top map[string]any
	if err := json.Unmarshal([]byte(body), &top); err != nil {
		return ""
	}
	// {"error":{"message":"..."}}
	if e, ok := top["error"]; ok {
		switch v := e.(type) {
		case string:
			return v
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				return m
			}
			if m, ok := v["msg"].(string); ok {
				return m
			}
			if t, ok := v["type"].(string); ok {
				return t
			}
		}
	}
	if m, ok := top["message"].(string); ok {
		return m
	}
	return ""
}

func looksUnsafeForUser(s string) bool {
	if s == "" {
		return true
	}
	if looksLikeJSON(s) || looksLikeStack(s) {
		return true
	}
	if len(s) > 200 {
		return true
	}
	return false
}

func looksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")
}

func looksLikeStack(s string) bool {
	return strings.Contains(s, "goroutine ") ||
		strings.Count(s, ".go:") >= 2 ||
		strings.Contains(s, "\n\t/") ||
		strings.Contains(s, "runtime.")
}

func sanitizeHint(s string) string {
	s = strings.TrimSpace(s)
	s = redactSecrets(s)
	if looksLikeStack(s) {
		return "See /doctor or provider status — details logged if enabled"
	}
	if looksLikeJSON(s) {
		if m := extractAPIMessage(s); m != "" {
			s = m
		} else {
			return "Retry, or run /doctor"
		}
	}
	// collapse whitespace
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 140 {
		s = s[:137] + "…"
	}
	return s
}

func redactSecrets(s string) string {
	// inline light redaction (avoid import cycle with redact package if any)
	re := regexp.MustCompile(`(?i)(api[_-]?key|token|secret|bearer|authorization)["'\s:=]+[^\s"',}{]+`)
	s = re.ReplaceAllString(s, "$1=[REDACTED]")
	re2 := regexp.MustCompile(`\b(sk-[A-Za-z0-9_\-]{10,}|xai-[A-Za-z0-9_\-]{10,}|AIza[A-Za-z0-9_\-]{10,}|sk-ant-[A-Za-z0-9_\-]{10,})\b`)
	s = re2.ReplaceAllString(s, "[REDACTED]")
	return s
}

// HTTPError builds a classified error from an HTTP response.
func HTTPError(provider string, status int, body []byte, transportErr error) error {
	return Classify(transportErr, status, string(body), provider)
}

// HTTPErrorHeaders is like HTTPError but honors Retry-After header seconds.
func HTTPErrorHeaders(provider string, status int, body []byte, hdr http.Header, transportErr error) error {
	pe := Classify(transportErr, status, string(body), provider)
	if pe.Code == ErrRateLimit && pe.RetryAfter == 0 && hdr != nil {
		if ra := hdr.Get("Retry-After"); ra != "" {
			if sec, err := strconv.Atoi(strings.TrimSpace(ra)); err == nil && sec > 0 {
				pe.RetryAfter = time.Duration(sec) * time.Second
				pe.Hint = fmt.Sprintf("Wait ~%s, or switch model (/model)", pe.RetryAfter.Round(time.Second))
			}
		}
	}
	return pe
}

// AuthError is a convenience constructor for missing-key ValidateConfig paths.
func AuthError(provider, message string) error {
	if message == "" {
		message = "API key rejected or missing"
	}
	return &ProviderError{
		Code: ErrAuth, Message: message, Hint: authHint(provider),
		Provider: provider, Retry: false,
	}
}

// ── optional error log (E6) ──────────────────────────────────────────

var (
	logOnce   sync.Once
	logPath   string
	logMu     sync.Mutex
	logEnable *bool
)

// LogPath returns ~/.codeforge/logs/provider-error.jsonl (created on first write).
func LogPath() string {
	logOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			logPath = ""
			return
		}
		logPath = filepath.Join(home, ".codeforge", "logs", "provider-error.jsonl")
	})
	return logPath
}

func loggingEnabled() bool {
	if logEnable != nil {
		return *logEnable
	}
	// default on; disable with CODEFORGE_PROVIDER_ERROR_LOG=0
	v := strings.ToLower(strings.TrimSpace(os.Getenv("CODEFORGE_PROVIDER_ERROR_LOG")))
	if v == "0" || v == "off" || v == "false" {
		return false
	}
	return true
}

func logProviderError(pe *ProviderError) {
	if pe == nil || !loggingEnabled() {
		return
	}
	// skip noisy cancels
	if pe.Code == ErrTimeout && pe.Message == "Request canceled" {
		return
	}
	path := LogPath()
	if path == "" {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	rec := map[string]any{
		"ts":       time.Now().UTC().Format(time.RFC3339),
		"code":     string(pe.Code),
		"message":  pe.Message,
		"hint":     pe.Hint,
		"status":   pe.Status,
		"provider": pe.Provider,
		"retry":    pe.Retry,
		"raw":      pe.Raw,
	}
	if pe.RetryAfter > 0 {
		rec["retry_after_sec"] = int(pe.RetryAfter.Seconds())
	}
	b, _ := json.Marshal(rec)
	_, _ = f.Write(append(b, '\n'))
}
