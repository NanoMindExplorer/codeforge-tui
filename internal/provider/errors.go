package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
type ProviderError struct {
	Code       ErrorCode
	Message    string        // short human message
	Hint       string        // next step
	Retry      bool          // safe to retry
	RetryAfter time.Duration // optional
	Status     int           // HTTP status if any
	Provider   string        // grok|openai|gemini|claude|ollama
	Raw        string        // truncated raw body for logs
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

// UserMessage is multi-line text safe for TUI system blocks.
func (e *ProviderError) UserMessage() string {
	if e == nil {
		return "⚠ Unknown provider error"
	}
	var b strings.Builder
	b.WriteString("⚠ ")
	b.WriteString(e.Message)
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
	return b.String()
}

// AsProviderError extracts *ProviderError from err chain.
func AsProviderError(err error) (*ProviderError, bool) {
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe, true
	}
	// wrap unknown
	if err == nil {
		return nil, false
	}
	return Classify(err, 0, "", ""), true
}

// FormatUserError turns any error into a friendly multi-line string.
func FormatUserError(err error) string {
	if err == nil {
		return ""
	}
	if pe, ok := err.(*ProviderError); ok {
		return pe.UserMessage()
	}
	var pe *ProviderError
	if errors.As(err, &pe) {
		return pe.UserMessage()
	}
	// best-effort classify string
	return Classify(err, 0, err.Error(), "").UserMessage()
}

// Classify maps transport/API failures to ProviderError.
func Classify(err error, status int, body, provider string) *ProviderError {
	raw := truncateRaw(body, 400)
	low := strings.ToLower(body + " " + errString(err))

	pe := &ProviderError{
		Code:     ErrUnknown,
		Status:   status,
		Provider: provider,
		Raw:      raw,
		Message:  "Provider request failed",
		Hint:     "Check network and API key, then retry",
		Retry:    true,
	}

	// Network / timeout
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			pe.Code = ErrTimeout
			pe.Message = "Request timed out"
			pe.Hint = "Retry, or raise timeout / use a faster model"
			pe.Retry = true
			return pe
		}
		if errors.Is(err, context.Canceled) || strings.Contains(low, "context canceled") {
			pe.Code = ErrTimeout
			pe.Message = "Request canceled"
			pe.Hint = "You stopped the turn, or a new message interrupted it"
			pe.Retry = false
			return pe
		}
		if strings.Contains(low, "connection refused") || strings.Contains(low, "no such host") ||
			strings.Contains(low, "network is unreachable") || strings.Contains(low, "tls") {
			pe.Code = ErrNetwork
			pe.Message = "Cannot reach the API"
			pe.Hint = "Check network, proxy, or base URL (XAI_BASE_URL / OPENAI_BASE_URL)"
			pe.Retry = true
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
		return pe
	case http.StatusTooManyRequests:
		pe.Code = ErrRateLimit
		pe.Message = "Rate limited by the provider"
		pe.Hint = "Wait a moment, lower concurrency, or switch model (/model)"
		pe.Retry = true
		if d := parseRetryAfter(body); d > 0 {
			pe.RetryAfter = d
		}
		return pe
	case http.StatusPaymentRequired:
		pe.Code = ErrQuota
		pe.Message = "Quota or billing limit reached"
		pe.Hint = "Check provider console billing / plan"
		pe.Retry = false
		return pe
	case http.StatusBadRequest:
		return classifyBadRequest(pe, low, provider)
	case http.StatusNotFound:
		if strings.Contains(low, "model") {
			pe.Code = ErrModel
			pe.Message = "Model not found"
			pe.Hint = "Run /model to pick a valid id"
			pe.Retry = false
			return pe
		}
	case http.StatusRequestEntityTooLarge:
		pe.Code = ErrContext
		pe.Message = "Request too large"
		pe.Hint = "Use /compact or shorten the conversation"
		pe.Retry = false
		return pe
	case 529, 503, 502:
		pe.Code = ErrNetwork
		pe.Message = "Provider temporarily unavailable"
		pe.Hint = "Retry in a few seconds"
		pe.Retry = true
		return pe
	}

	// Body heuristics (when status missing or 200-wrapped errors)
	switch {
	case containsAny(low, "invalid api key", "incorrect api key", "authentication", "unauthorized", "api key not set", "not set"):
		pe.Code = ErrAuth
		pe.Message = "API key rejected or missing"
		pe.Hint = authHint(provider)
		pe.Retry = false
	case containsAny(low, "rate limit", "rate_limit", "too many requests", "quota exceeded", "resource_exhausted"):
		if containsAny(low, "billing", "insufficient_quota", "credit") {
			pe.Code = ErrQuota
			pe.Message = "Quota or billing limit reached"
			pe.Hint = "Check provider console billing / plan"
			pe.Retry = false
		} else {
			pe.Code = ErrRateLimit
			pe.Message = "Rate limited by the provider"
			pe.Hint = "Wait a moment or switch model (/model)"
			pe.Retry = true
		}
	case containsAny(low, "context_length", "maximum context", "too many tokens", "token limit", "max_tokens", "context window"):
		pe.Code = ErrContext
		pe.Message = "Context too long for this model"
		pe.Hint = "Run /compact or start /new session"
		pe.Retry = false
	case containsAny(low, "thinking", "reasoning", "include_reasoning", "reasoning_effort", "budget_tokens"):
		pe.Code = ErrUnsupported
		pe.Message = "Reasoning/thinking not supported for this request"
		pe.Hint = "Retry without thinking (CODEFORGE_REASONING=off) or pick another model"
		pe.Retry = true
	case containsAny(low, "model", "does not exist", "not found", "unknown model"):
		pe.Code = ErrModel
		pe.Message = "Model not available"
		pe.Hint = "Check /model list or provider catalog"
		pe.Retry = false
	case containsAny(low, "timeout", "deadline exceeded"):
		pe.Code = ErrTimeout
		pe.Message = "Request timed out"
		pe.Hint = "Retry or use a faster model"
		pe.Retry = true
	default:
		if err != nil {
			// keep short
			msg := err.Error()
			if len(msg) > 160 {
				msg = msg[:157] + "…"
			}
			pe.Message = msg
		}
		if status > 0 {
			pe.Message = fmt.Sprintf("Provider error (HTTP %d)", status)
			if raw != "" {
				// one line from raw
				line := strings.ReplaceAll(raw, "\n", " ")
				if len(line) > 120 {
					line = line[:117] + "…"
				}
				pe.Hint = line
			}
		}
	}
	return pe
}

func classifyBadRequest(pe *ProviderError, low, provider string) *ProviderError {
	switch {
	case containsAny(low, "thinking", "reasoning", "include_reasoning", "budget_tokens"):
		pe.Code = ErrUnsupported
		pe.Message = "This model rejected reasoning/thinking parameters"
		pe.Hint = "Set CODEFORGE_REASONING=off or choose a thinking-capable model"
		pe.Retry = true
	case containsAny(low, "context", "token", "too long", "maximum"):
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
		pe.Hint = "Check model id and request options"
		pe.Retry = false
	}
	_ = provider
	return pe
}

func authHint(provider string) string {
	switch strings.ToLower(provider) {
	case "grok", "xai":
		return "Set XAI_API_KEY or GROK_API_KEY, then /provider grok"
	case "gemini":
		return "Set GEMINI_API_KEY (https://aistudio.google.com/apikey)"
	case "claude", "anthropic":
		return "Set ANTHROPIC_API_KEY"
	case "openai":
		return "Set OPENAI_API_KEY (and OPENAI_BASE_URL if using a proxy)"
	case "ollama":
		return "Start Ollama locally (ollama serve) and pull a model"
	default:
		return "Run /setup or set the provider API key, then /provider"
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
	// try JSON retry_after seconds
	re := regexp.MustCompile(`(?i)"retry[_-]after"\s*:\s*(\d+)`)
	if m := re.FindStringSubmatch(body); len(m) == 2 {
		if sec, err := strconv.Atoi(m[1]); err == nil && sec > 0 {
			return time.Duration(sec) * time.Second
		}
	}
	return 0
}

// HTTPError builds a classified error from an HTTP response.
func HTTPError(provider string, status int, body []byte, transportErr error) error {
	return Classify(transportErr, status, string(body), provider)
}
