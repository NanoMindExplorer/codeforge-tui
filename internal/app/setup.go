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
	"github.com/codeforge/tui/internal/plugin"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/research"
	"github.com/codeforge/tui/internal/rules"
	"github.com/codeforge/tui/internal/telemetry"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/workspace"
)

// Runtime is a fully wired CodeForge environment.
type Runtime struct {
	Cfg      *config.Config
	WorkDir  string
	ProvReg  *provider.Registry
	ToolReg  *tool.Registry
	GitRepo  *git.Repo
	Rules    *rules.Bundle
	Quiet    bool // suppress stderr banners
	Tele     *telemetry.Client
}

// Options controls bootstrap behaviour.
type Options struct {
	WorkDir     string
	Quiet       bool
	SkipIndex   bool
	SkipMCP     bool
	SkipPlugins bool
	ActMode     bool // force Act write mode (headless default often Act)
	PlanMode    bool // force Plan
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
		cfg = config.Default()
	}
	_ = config.SaveExample()

	logf := func(format string, args ...any) {
		if opt.Quiet {
			return
		}
		fmt.Fprintf(os.Stderr, format, args...)
	}

	provReg := provider.NewRegistry()
	registerProviders(provReg, logf)

	if _, err := provReg.Current(); err != nil {
		_ = provReg.Register(provider.NewClaudeProvider("", "claude-sonnet-4-20250514"))
	}
	// Provider preference: explicit env keys win, then config default
	switch {
	case os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "":
		_ = provReg.Switch("grok")
	case os.Getenv("GEMINI_API_KEY") != "":
		_ = provReg.Switch("gemini")
	case cfg.DefaultProvider != "":
		_ = provReg.Switch(cfg.DefaultProvider)
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
		ch := agent.Run(ctx, agent.Config{
			Provider: p, Tools: tools, System: system,
			MaxTokens: 2048, MaxIterations: maxIter,
		}, msgs)
		for ev := range ch {
			switch ev.Kind {
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
	// Prefer Grok 4.5 when XAI key is present
	if k := os.Getenv("XAI_API_KEY"); k != "" || os.Getenv("GROK_API_KEY") != "" {
		_ = reg.Register(provider.NewGrokProvider(k, "grok-4.5"))
		logf("✓ Grok (xAI) registered — model grok-4.5\n")
		_ = reg.Switch("grok")
	}
	if k := os.Getenv("GEMINI_API_KEY"); k != "" {
		_ = reg.Register(provider.NewGeminiProvider(k, "gemini-2.5-flash"))
		logf("✓ Gemini registered\n")
	}
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		_ = reg.Register(provider.NewClaudeProvider(k, "claude-sonnet-4-20250514"))
		logf("✓ Claude registered\n")
	}
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		_ = reg.Register(provider.NewOpenAIProvider(k, "gpt-4o-mini"))
		logf("✓ OpenAI registered\n")
	}
	ollama := provider.NewOllamaProvider("")
	if err := ollama.ValidateConfig(); err == nil {
		_ = reg.Register(ollama)
		logf("✓ Ollama registered (local)\n")
	}
}
