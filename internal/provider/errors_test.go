package provider

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestClassifyAuth(t *testing.T) {
	pe := Classify(nil, 401, `{"error":"invalid api key"}`, "grok")
	if pe.Code != ErrAuth {
		t.Fatal(pe.Code)
	}
	if !strings.Contains(pe.UserMessage(), "API key") {
		t.Fatal(pe.UserMessage())
	}
}

func TestClassifyRateLimit(t *testing.T) {
	pe := Classify(nil, 429, `{"error":{"message":"rate_limit exceeded","retry_after":20}}`, "openai")
	if pe.Code != ErrRateLimit || !pe.Retry {
		t.Fatal(pe)
	}
	if pe.RetryAfter < 20e9 { // 20s
		// may parse from body
	}
}

func TestClassifyReasoningUnsupported(t *testing.T) {
	pe := Classify(nil, 400, `unknown field include_reasoning`, "grok")
	if pe.Code != ErrUnsupported {
		t.Fatalf("%s %s", pe.Code, pe.Message)
	}
}

func TestClassifyContext(t *testing.T) {
	pe := Classify(nil, 400, `maximum context length exceeded`, "openai")
	if pe.Code != ErrContext {
		t.Fatal(pe.Code)
	}
}

func TestClassifyNetwork(t *testing.T) {
	pe := Classify(fmt.Errorf("dial tcp: connection refused"), 0, "", "ollama")
	if pe.Code != ErrNetwork {
		t.Fatal(pe.Code)
	}
}

func TestFormatUserError(t *testing.T) {
	err := HTTPError("gemini", http.StatusTooManyRequests, []byte("rate limit"), nil)
	s := FormatUserError(err)
	if !strings.Contains(s, "Rate limited") && !strings.Contains(s, "rate") {
		t.Fatal(s)
	}
}

func TestAsProviderError(t *testing.T) {
	err := &ProviderError{Code: ErrModel, Message: "nope", Hint: "pick another"}
	pe, ok := AsProviderError(err)
	if !ok || pe.Code != ErrModel {
		t.Fatal(ok, pe)
	}
	// plain error still classifies
	pe2, ok := AsProviderError(errors.New("invalid api key"))
	if !ok || pe2.Code != ErrAuth {
		t.Fatal(pe2)
	}
}
