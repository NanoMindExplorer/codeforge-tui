// Package doctor runs environment health checks (W4 / E7).
package doctor

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/onboarding"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/sandbox"
	"github.com/codeforge/tui/internal/theme"
)

// Report is a multi-section health dump safe for TUI / CLI.
type Report struct {
	Lines  []string
	OK     bool
	Issues int
}

// Options configure which subsystems to inspect.
type Options struct {
	Registry *provider.Registry
	WorkDir  string
	Version  string
}

// Run builds a doctor report.
func Run(opt Options) Report {
	var lines []string
	issues := 0
	add := func(ok bool, format string, args ...any) {
		prefix := "✓"
		if !ok {
			prefix = "✗"
			issues++
		}
		lines = append(lines, fmt.Sprintf("%s "+format, append([]any{prefix}, args...)...))
	}
	info := func(format string, args ...any) {
		lines = append(lines, "· "+fmt.Sprintf(format, args...))
	}

	lines = append(lines, "CodeForge doctor")
	if opt.Version != "" {
		info("version %s", opt.Version)
	}
	info("go %s · %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	if opt.WorkDir != "" {
		info("workdir %s", opt.WorkDir)
	}
	lines = append(lines, "")

	// Theme / terminal
	lines = append(lines, "Terminal")
	lvl := theme.DetectColorLevel()
	add(true, "color level: %s (CODEFORGE_COLOR=%q NO_COLOR=%q)",
		theme.ColorLevelName(lvl), os.Getenv("CODEFORGE_COLOR"), os.Getenv("NO_COLOR"))
	info("theme: %s · minimal=%v · compact=%v · motion=%v",
		theme.Name(), theme.MinimalMode(), theme.CompactMode(), theme.MotionEnabled())
	info("TERM=%s COLORTERM=%s", os.Getenv("TERM"), os.Getenv("COLORTERM"))
	lines = append(lines, "")

	// Providers / keys
	lines = append(lines, "Providers")
	cfg, _ := config.Load()
	res := onboarding.ResolveActive(cfg)
	if res.Provider != "" {
		info("resolution: %s — %s", res.Provider, res.Reason)
		if len(res.Alternatives) > 0 {
			info("also available: %s", strings.Join(res.Alternatives, ", "))
		}
	}
	if opt.Registry == nil {
		add(false, "provider registry not wired")
	} else {
		list := opt.Registry.List()
		if len(list) == 0 {
			add(false, "no providers registered")
		} else {
			info("registered: %s", strings.Join(list, ", "))
		}
		cur, err := opt.Registry.Current()
		if err != nil {
			add(false, "current provider: %v", err)
		} else {
			name := opt.Registry.CurrentName()
			src, _ := onboarding.KeySource(name)
			if name == "ollama" {
				src = "local"
			}
			if err := cur.ValidateConfig(); err != nil {
				add(false, "%s validate: %v (key: %s)", name, err, src)
			} else {
				add(true, "%s ok · model %s · key %s", name, cur.Model(), src)
			}
			models := cur.Models()
			if len(models) > 0 {
				n := len(models)
				if n > 5 {
					n = 5
				}
				var ids []string
				for i := 0; i < n; i++ {
					ids = append(ids, models[i].ID)
				}
				info("models (sample): %s", strings.Join(ids, ", "))
			}
		}
		// Key matrix
		for _, name := range []string{"grok", "gemini", "claude", "openai"} {
			src, ok := onboarding.KeySource(name)
			mark := "○"
			if ok {
				mark = "✓"
			}
			info("%s %-8s %s", mark, name, src)
		}
		nKeys := onboarding.CountPresentKeys()
		if nKeys > 1 {
			info("multi-key mode: %d cloud keys — active is sticky via /provider", nKeys)
		}
		if !onboarding.HasAnyAPIKey() && !onboarding.ProviderHealthy(opt.Registry) {
			add(false, "no API key configured — run /setup")
		}
	}
	lines = append(lines, "")

	// Sandbox
	lines = append(lines, "Sandbox")
	eng := sandbox.Global()
	if eng == nil {
		info("engine: default/off")
	} else {
		add(true, "%s", eng.Summary())
		info("label: %s · bwrap=%v", eng.Label(), sandbox.HasBubblewrap())
	}
	lines = append(lines, "")

	// Env tips
	lines = append(lines, "Hints")
	if issues == 0 {
		info("all critical checks passed")
	} else {
		info("%d issue(s) — /setup for keys, /sandbox for isolation, docs/TERMINAL_MATRIX.md for color", issues)
	}
	info("headless CI: codeforge agent --json \"…\"  (exit 2 = no_provider/auth)")
	info("release gate: make release-gate")

	return Report{Lines: lines, OK: issues == 0, Issues: issues}
}

// String renders the report.
func (r Report) String() string {
	return strings.Join(r.Lines, "\n")
}
