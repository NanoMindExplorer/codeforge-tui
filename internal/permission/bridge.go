package permission

import (
	"context"

	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/hooks"
)

// FromConfig builds an Engine from config + loads hooks + remembered grants.
func FromConfig(cfg *config.Config, workdir string) *Engine {
	e := NewEngine(workdir)
	if cfg != nil {
		if cfg.Permissions.Mode != "" {
			e.Mode = ParseMode(cfg.Permissions.Mode)
		} else if !cfg.Permissions.RequireConfirmWrite {
			// legacy: no confirm write → always approve vibe
			e.Mode = ModeAlwaysApprove
		}
		for _, r := range cfg.Permissions.Rules {
			eff := Effect(r.Effect)
			if eff != EffectDeny && eff != EffectAsk && eff != EffectAllow {
				continue
			}
			e.Rules = append(e.Rules, Rule{Tool: r.Tool, Pattern: r.Pattern, Effect: eff})
		}
	}
	e.LoadRemembered()

	// Attach hooks
	hr := hooks.Load(workdir)
	if hr != nil && hr.Count() > 0 {
		hrCopy := hr
		e.PreHooks = append(e.PreHooks, func(ctx context.Context, tool, input string) (bool, string) {
			return hrCopy.PreToolUse(ctx, tool, input)
		})
		e.PostHooks = append(e.PostHooks, func(ctx context.Context, tool, input, output string, success bool) {
			hrCopy.PostToolUse(ctx, tool, input, output, success)
		})
	}
	return e
}

// AttachHooks adds a hooks runner to an existing engine.
func (e *Engine) AttachHooks(hr *hooks.Runner) {
	if e == nil || hr == nil || hr.Count() == 0 {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.PreHooks = append(e.PreHooks, func(ctx context.Context, tool, input string) (bool, string) {
		return hr.PreToolUse(ctx, tool, input)
	})
	e.PostHooks = append(e.PostHooks, func(ctx context.Context, tool, input, output string, success bool) {
		hr.PostToolUse(ctx, tool, input, output, success)
	})
}
