// Package app boots shared CodeForge runtime (TUI and headless).
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codeforge/tui/internal/agent"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/git"
	"github.com/codeforge/tui/internal/index"
	"github.com/codeforge/tui/internal/onboarding"
	"github.com/codeforge/tui/internal/pager"
	"github.com/codeforge/tui/internal/personas"
	"github.com/codeforge/tui/internal/plugin"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/research"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/sandbox"
	"github.com/codeforge/tui/internal/skills"
	"github.com/codeforge/tui/internal/telemetry"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/workspace"
)

// Runtime is a fully wired CodeForge environment.
type Runtime struct {
	Cfg     *config.Config
	WorkDir string
	ProvReg *provider.Registry
	ToolReg *tool.Registry
	GitRepo *git.Repo
	Rules   *rules.Bundle
	Quiet   bool // suppress stderr banners
	Tele    *telemetry.Client
}

// Options controls bootstrap behaviour.
type Options struct {
	WorkDir string
	Quiet   bool
	// SkipIndex skips codebase index build (Q3.4). Headless/ACP default true;
	// interactive TUI leaves this false so search tools see a fresh index.
	// Override at runtime with CODEFORGE_INDEX=1 (force index) when SkipIndex
	// would otherwise be true — callers check env themselves, or set SkipIndex.
	SkipIndex   bool
	SkipMCP     bool
	SkipPlugins bool
	ActMode     bool // force Act write mode (headless default often Act)
	PlanMode    bool // force Plan
	// Sandbox profile override (empty = use config / env).
	Sandbox        string
	SandboxFlagSet bool // true when CLI --sandbox was passed
}

// Bootstrap loads config, providers, tools, index, plugins, MCP.
func Bootstrap(opt Options) (*Runtime, error) {
	workdir := opt.WorkDir
	if workdir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		workdir = wd
	}
	if abs, err := filepath.Abs(workdir); err == nil {
		workdir = abs
	}

	cfg, err := config.Load()
	if err != nil {
		// Schema errors should be visible; still boot with defaults so --help-like paths work.
		if !opt.Quiet {
			fmt.Fprintf(os.Stderr, "Warning: config: %v\n  → using defaults; fix ~/.config/codeforge/config.yaml\n", err)
		}
		cfg = config.Default()
	}
	_ = config.SaveExample()

	logf := func(format string, args ...any) {
		if opt.Quiet {
			return
		}
		fmt.Fprintf(os.Stderr, format, args...)
	}

	// Force index when ops set CODEFORGE_INDEX=1 even if caller skipped.
	if os.Getenv("CODEFORGE_INDEX") == "1" {
		opt.SkipIndex = false
	}

	provReg := provider.NewRegistry()
	registerProvidersWithConfig(provReg, cfg, logf)

	if _, err := provReg.Current(); err != nil {
		_ = provReg.Register(provider.NewClaudeProvider("", "claude-sonnet-4-20250514"))
	}
	// Multi-provider resolution (shared with wizard / /provider): preference → config → priority order
	res := onboarding.ResolveActive(cfg)
	if res.Provider != "" {
		if err := provReg.Switch(res.Provider); err != nil {
			// preferred not registered (e.g. ollama down) — leave whatever was registered
			logf("Note: could not activate %s (%v) — %s\n", res.Provider, err, res.Reason)
		} else {
			if res.Model != "" {
				if p, err := provReg.Current(); err == nil {
					_ = p.SetModel(res.Model)
				}
			}
			logf("✓ Active provider: %s (%s)\n", res.Provider, res.Reason)
			if len(res.Alternatives) > 0 {
				logf("  other keys: %v — switch with /provider\n", res.Alternatives)
			}
		}
	}

	ws := workspace.New(workdir)
	for _, root := range cfg.Workspace.ExtraRoots {
		if err := ws.AddRoot(root, ""); err != nil {
			logf("Warning: workspace root %s: %v\n", root, err)
		} else {
			logf("✓ Workspace root: %s\n", root)
		}
	}
	if len(cfg.Workspace.IgnoreDirs) > 0 {
		ws.SetIgnoreDirs(cfg.Workspace.IgnoreDirs)
	}
	workspace.SetGlobal(ws)

	// Grok pager.toml / pager.yaml (layout, scrollbar, blocks, animation, ui knobs)
	pg := pager.ApplyFromWorkdir(workdir)
	// Overlay config.yaml [ui] knobs into pager when set
	pg = pager.MergeConfigUI(pg, cfg)
	pager.Apply(pg)
	if pg.Source != "" {
		logf("✓ %s\n", pg.Summary())
	}
	// Apply UI knobs from pager into config-ish flags
	if pg.UI.VimMode != nil {
		cfg.UI.VimMode = *pg.UI.VimMode
	}
	if pg.UI.CompactMode != nil {
		cfg.UI.CompactMode = *pg.UI.CompactMode
	}

	// Phase G4: Grok-compatible shell sandbox
	prof := sandbox.ResolvePreferExplicit(opt.SandboxFlagSet, opt.Sandbox, cfg.Sandbox.Profile)
	eng := sandbox.Ensure(prof, workdir)
	if len(cfg.Sandbox.Deny) > 0 {
		eng.Deny = append([]string(nil), cfg.Sandbox.Deny...)
	}
	if !eng.IsOff() {
		logf("✓ %s\n", eng.Summary())
		sandbox.LogEvent("activate", map[string]any{
			"profile": string(eng.Profile),
			"backend": string(eng.Backend),
			"process": string(eng.ProcessBackend),
			"workdir": workdir,
		})
	}

	// Load persisted subagent jobs (cross-session)
	if n, err := tool.SubJobs.LoadFromDisk(); err == nil && n > 0 {
		logf("✓ subagent jobs: restored %d from disk\n", n)
	}

	var extra []string
	for _, r := range ws.ListRoots() {
		if r.Path != workdir {
			extra = append(extra, r.Path)
		}
	}
	rb := rules.Load(workdir, extra...)
	if len(rb.Paths) > 0 {
		logf("✓ %s\n", rb.Summary())
	}

	// Phase G5: Grok-compatible skills
	skReg := skills.Load(skills.Options{
		WorkDir:      workdir,
		ExtraPaths:   cfg.Skills.Paths,
		Ignore:       cfg.Skills.Ignore,
		Disabled:     cfg.Skills.Disabled,
		CompatClaude: cfg.SkillsCompatClaude(),
		CompatCursor: cfg.SkillsCompatCursor(),
	})
	if skReg.Count() > 0 {
		logf("✓ %s\n", skReg.Summary())
	}

	// Phase G6: personas for spawn_subagent
	cfgPersonas := map[string]personas.Persona{}
	for name, sp := range cfg.Subagents.Personas {
		cfgPersonas[name] = personas.Persona{
			Name:             name,
			Description:      sp.Description,
			Instructions:     sp.Instructions,
			InstructionsFile: sp.InstructionsFile,
			Model:            sp.Model,
			DefaultIsolation: sp.DefaultIsolation,
		}
	}
	pReg := personas.Load(personas.Options{
		WorkDir:        workdir,
		ConfigPersonas: cfgPersonas,
		ExtraDirs:      cfg.Subagents.ExtraDirs,
	})
	if pReg.Count() > 0 {
		logf("✓ %s\n", pReg.Summary())
	}

	if !opt.SkipIndex {
		if idx, err := index.Build(workdir); err != nil {
			logf("Warning: index: %v\n", err)
		} else {
			index.SetGlobal(idx)
			f, s := idx.Stats()
			logf("✓ Codebase index: %d files, %d symbols\n", f, s)
		}
	}

	toolReg := tool.NewRegistry(workdir)
	toolReg.Register(&research.Tool{WorkDir: workdir, Parent: toolReg, ProvReg: provReg})
	// Wire Grok-compatible spawn_subagent (avoids tool↔agent import cycle)
	tool.SubagentParentRegistry = toolReg
	tool.SubagentRunner = func(ctx context.Context, workdir, system string, msgs []provider.Message, tools *tool.Registry, maxIter int, onEvent func(tool.SubagentEvent)) {
		p, err := provReg.Current()
		if err != nil {
			onEvent(tool.SubagentEvent{Kind: "error", Error: err.Error()})
			return
		}
		// Prefer tools.Authorizer (per-session ACP) over process-wide SubagentAuthorizer (Q6.2).
		var auth agent.Authorizer
		if a := tool.ResolveSubagentAuth(tools); a != nil {
			auth = subAuthBridge{a}
		}
		ch := agent.Run(ctx, agent.Config{
			Provider: p, Tools: tools, System: system,
			MaxTokens: 2048, MaxIterations: maxIter, Auth: auth,
		}, msgs)
		for ev := range ch {
			switch ev.Kind {
			case agent.EventThinking:
				if ev.Thinking != "" {
					onEvent(tool.SubagentEvent{Kind: "thinking", Text: ev.Thinking})
				}
			case agent.EventText:
				onEvent(tool.SubagentEvent{Kind: "text", Text: ev.Text})
			case agent.EventToolCall:
				onEvent(tool.SubagentEvent{Kind: "tool_call", ToolName: ev.ToolName})
			case agent.EventError:
				if ev.Error != nil {
					onEvent(tool.SubagentEvent{Kind: "error", Error: ev.Error.Error()})
				}
			case agent.EventDone:
				onEvent(tool.SubagentEvent{Kind: "done"})
			}
		}
	}

	// Write mode
	if sw := toolReg.GetStagedWriter(); sw != nil {
		switch {
		case opt.ActMode:
			sw.SetMode(tool.ModeAct)
		case opt.PlanMode:
			sw.SetMode(tool.ModePlan)
		case cfg.Permissions.RequireConfirmWrite:
			sw.SetMode(tool.ModePlan)
		default:
			sw.SetMode(tool.ModeAct)
		}
	}

	if !opt.SkipMCP && len(cfg.MCP.Servers) > 0 {
		var servers []provider.MCPServerConfig
		for _, s := range cfg.MCP.Servers {
			servers = append(servers, provider.MCPServerConfig{
				Name: s.Name, Command: s.Command, Args: s.Args, Env: s.Env,
			})
		}
		for _, line := range tool.RegisterMCPServers(toolReg, servers) {
			logf("✓ %s\n", line)
		}
	}

	if !opt.SkipPlugins {
		n, lines := plugin.LoadAll(toolReg, workdir, cfg.Plugins.Dirs)
		for _, line := range lines {
			logf("%s\n", line)
		}
		if n > 0 {
			logf("✓ Plugins: %d tool(s)\n", n)
		}
	}

	repo, err := git.Open(workdir)
	if err != nil {
		logf("Warning: git: %v\n", err)
		repo = nil
	}

	tele := telemetry.New(telemetry.Config{
		Enabled:   cfg.Telemetry.Enabled,
		Endpoint:  cfg.Telemetry.Endpoint,
		LocalOnly: cfg.Telemetry.LocalOnly,
	})
	tele.Event("boot", map[string]any{"workdir": filepath.Base(workdir)})

	if cfg.Budget.MaxCostUSD > 0 {
		logf("✓ Budget cap: $%.2f\n", cfg.Budget.MaxCostUSD)
	}

	return &Runtime{
		Cfg:     cfg,
		WorkDir: workdir,
		ProvReg: provReg,
		ToolReg: toolReg,
		GitRepo: repo,
		Rules:   rb,
		Quiet:   opt.Quiet,
		Tele:    tele,
	}, nil
}

func registerProviders(reg *provider.Registry, logf func(string, ...any)) {
	registerProvidersWithConfig(reg, nil, logf)
}

func registerProvidersWithConfig(reg *provider.Registry, cfg *config.Config, logf func(string, ...any)) {
	// Helper: env wins over config key
	keyOf := func(envKeys []string, cfgKey string) string {
		for _, e := range envKeys {
			if v := os.Getenv(e); v != "" {
				return v
			}
		}
		return cfgKey
	}
	cfgKey := func(name string) (apiKey, model, endpoint string) {
		if cfg == nil || cfg.Providers == nil {
			return "", "", ""
		}
		if p, ok := cfg.Providers[name]; ok {
			return p.APIKey, p.DefaultModel, p.Endpoint
		}
		return "", "", ""
	}

	// Grok
	gk, gm, ge := cfgKey("grok")
	if gk == "" {
		gk, _, _ = cfgKey("xai")
	}
	gk = keyOf([]string{"XAI_API_KEY", "GROK_API_KEY"}, gk)
	if gm == "" {
		gm = "grok-4.5"
	}
	if gk != "" {
		p := provider.NewGrokProvider(gk, gm)
		if ge != "" {
			// OpenAI-compatible endpoint override via env already; set if exposed
			_ = ge
		}
		_ = reg.Register(p)
		logf("✓ Grok (xAI) registered — model %s\n", gm)
		_ = reg.Switch("grok")
	}

	// Gemini
	gemK, gemM, _ := cfgKey("gemini")
	gemK = keyOf([]string{"GEMINI_API_KEY"}, gemK)
	if gemM == "" {
		gemM = "gemini-2.5-flash"
	}
	if gemK != "" {
		_ = reg.Register(provider.NewGeminiProvider(gemK, gemM))
		logf("✓ Gemini registered\n")
	}

	// Claude
	ck, cm, _ := cfgKey("claude")
	ck = keyOf([]string{"ANTHROPIC_API_KEY"}, ck)
	if cm == "" {
		cm = "claude-sonnet-4-20250514"
	}
	if ck != "" {
		_ = reg.Register(provider.NewClaudeProvider(ck, cm))
		logf("✓ Claude registered\n")
	}

	// OpenAI
	ok, om, _ := cfgKey("openai")
	ok = keyOf([]string{"OPENAI_API_KEY"}, ok)
	if om == "" {
		om = "gpt-4o-mini"
	}
	if ok != "" {
		_ = reg.Register(provider.NewOpenAIProvider(ok, om))
		logf("✓ OpenAI registered\n")
	}

	ollama := provider.NewOllamaProvider("")
	if err := ollama.ValidateConfig(); err == nil {
		_ = reg.Register(ollama)
		logf("✓ Ollama registered (local)\n")
	}
}

// subAuthBridge adapts tool.SubagentAuth to agent.Authorizer.
type subAuthBridge struct{ a tool.SubagentAuth }

func (b subAuthBridge) Authorize(ctx context.Context, toolName, input string) error {
	if b.a == nil {
		return nil
	}
	return b.a.Authorize(ctx, toolName, input)
}

func (b subAuthBridge) NotifyPost(ctx context.Context, toolName, input, output string, success bool) {
	if b.a != nil {
		b.a.NotifyPost(ctx, toolName, input, output, success)
	}
}
