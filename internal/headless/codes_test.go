package headless

import (
	"errors"
	"testing"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/provider"
)

func TestMapErrorCodes(t *testing.T) {
	cases := []struct {
		err  error
		code string
	}{
		{&provider.ProviderError{Code: provider.ErrAuth, Message: "no key"}, "auth"},
		{&provider.ProviderError{Code: provider.ErrRateLimit, Message: "slow"}, "rate_limit"},
		{&provider.ProviderError{Code: provider.ErrQuota, Message: "bill"}, "quota"},
		{&provider.ProviderError{Code: provider.ErrModel, Message: "nope"}, "model"},
		{&provider.ProviderError{Code: provider.ErrContext, Message: "big"}, "context"},
		{&provider.ProviderError{Code: provider.ErrNetwork, Message: "down"}, "network"},
		{&provider.ProviderError{Code: provider.ErrTimeout, Message: "wait"}, "timeout"},
		{&provider.ProviderError{Code: provider.ErrUnsupported, Message: "think"}, "unsupported"},
		{&agent.LoopError{Code: "max_iterations", Message: "max", Hint: "raise"}, "max_iterations"},
		{&agent.LoopError{Code: "canceled", Message: "stop", Hint: "ok"}, "canceled"},
		{&agent.LoopError{Code: "no_provider", Message: "none", Hint: "setup"}, "no_provider"},
		{errors.New("weird"), "unknown"},
	}
	for _, c := range cases {
		code, msg, hint := mapAgentError(c.err)
		if code != c.code {
			t.Fatalf("%v: got code %q want %q (msg=%q hint=%q)", c.err, code, c.code, msg, hint)
		}
		if msg == "" {
			t.Fatalf("empty message for %v", c.err)
		}
	}
}
