// CodeForge TUI - Terminal AI Coding Companion
// Author: NanoMind (2026)
// License: Apache 2.0
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/acp"
	"github.com/codeforge/tui/internal/app"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/headless"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tui"
)

const (
	ProjectName    = "CodeForge TUI"
	ProjectVersion = "1.8.2"
	ProjectAuthor  = "NanoMind"
	ProjectYear    = "2026"
	ProjectLicense = "Apache 2.0"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		runTUI(args)
		return
	}

	switch args[0] {
	case "agent", "run":
		os.Exit(runAgentCLI(args[1:]))
	case "session":
		os.Exit(runSessionCLI(args[1:]))
	case "version", "--version", "-v":
		fmt.Printf("codeforge %s\n", ProjectVersion)
	case "help", "--help", "-h":
		printUsage()
	default:
		// flags or workdir → TUI
		runTUI(args)
	}
}

func runTUI(args []string) {
	noMotion := false
	skipWizard := false
	minimal := false
	compact := false
	sandboxFlag := ""
	sandboxSet := false
	var pathArgs []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--no-motion":
			noMotion = true
		case a == "--minimal":
			minimal = true
		case a == "--compact":
			compact = true
		case a == "--skip-wizard", a == "--yes", a == "-y":
			skipWizard = true
		case a == "--sandbox" || strings.HasPrefix(a, "--sandbox="):
			sandboxSet = true
			if strings.HasPrefix(a, "--sandbox=") {
				sandboxFlag = strings.TrimPrefix(a, "--sandbox=")
			} else if i+1 < len(args) {
				i++
				sandboxFlag = args[i]
			}
		case a == "--help" || a == "-h":
			printUsage()
			return
		case a == "--version" || a == "-v":
			fmt.Printf("codeforge %s\n", ProjectVersion)
			return
		default:
			if !strings.HasPrefix(a, "-") {
				pathArgs = append(pathArgs, a)
			}
		}
	}

	theme.InitFromEnv()
	if minimal {
		theme.SetMinimal(true)
	}
	if compact {
		theme.SetCompact(true)
	}
	if noMotion {
		theme.SetMotion(false)
	}

	workdir, _ := os.Getwd()
	if len(pathArgs) > 0 {
		if abs, err := filepath.Abs(pathArgs[0]); err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				workdir = abs
			}
		}
	}

	// Wizard before bootstrap if needed
	cfg, _ := config.Load()
	if cfg == nil {
		cfg = config.Default()
	}
	// Config theme (after env; --minimal wins)
	if !theme.MinimalMode() {
		themeName := cfg.Theme
		if cfg.UI.Theme != "" {
			themeName = cfg.UI.Theme
		}
		theme.ApplyFromConfig(themeName, cfg.UI.CompactMode, cfg.UI.AutoDarkTheme, cfg.UI.AutoLightTheme)
	}
	if !skipWizard && needsWizardQuick() {
		runWizard()
	}

	rt, err := app.Bootstrap(app.Options{
		WorkDir:        workdir,
		Sandbox:        sandboxFlag,
		SandboxFlagSet: sandboxSet,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if rt.Tele != nil {
		rt.Tele.Event("tui_start", nil)
		defer rt.Tele.Flush()
	}

	if cur, err := rt.ProvReg.Current(); err == nil {
		if err := cur.ValidateConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "\n⚠️  Provider config: %v\n", err)
			fmt.Fprintf(os.Stderr, "   Gemini free: https://aistudio.google.com/apikey\n\n")
		}
	}

	// OSC 12: cursor → accent_user for the session
	theme.ApplyCursorColor()
	defer theme.ResetCursorColor()

	model := tui.New(rt.Cfg, rt.ProvReg, rt.ToolReg, rt.GitRepo, rt.WorkDir)
	if !theme.MinimalMode() {
		printBanner()
	}

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runAgentCLI(args []string) int {
	// Shared flags for agent modes (before subcommand or task)
	acpOpt := acp.Options{
		Version: ProjectVersion,
		Quiet:   true,
		MaxIter: 12,
	}
	opt := headless.Options{
		JSON:    false,
		Act:     true,
		Timeout: 10 * time.Minute,
		MaxIter: 12,
	}
	var taskParts []string
	mode := "" // "", "stdio", "serve"
	bind := "127.0.0.1:2419"
	secret := ""

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "stdio":
			mode = "stdio"
		case a == "serve":
			mode = "serve"
		case a == "--json" || a == "-j":
			opt.JSON = true
			opt.Quiet = true
		case a == "--plan":
			opt.Plan = true
			opt.Act = false
			acpOpt.Plan = true
		case a == "--act":
			opt.Act = true
			opt.Plan = false
		case a == "--always-approve" || a == "--yolo":
			opt.AlwaysApprove = true
			opt.Act = true
			opt.Plan = false
			acpOpt.AlwaysApprove = true
		case a == "--dont-ask":
			opt.DontAsk = true
			acpOpt.DontAsk = true
		case a == "--sandbox" || strings.HasPrefix(a, "--sandbox="):
			opt.SandboxFlagSet = true
			if strings.HasPrefix(a, "--sandbox=") {
				opt.Sandbox = strings.TrimPrefix(a, "--sandbox=")
			} else if i+1 < len(args) {
				i++
				opt.Sandbox = args[i]
			}
		case a == "--model" || a == "-m":
			if i+1 < len(args) {
				i++
				opt.Model = args[i]
				acpOpt.Model = args[i]
			}
		case a == "--quiet" || a == "-q":
			opt.Quiet = true
		case a == "--workdir" || a == "-C":
			if i+1 < len(args) {
				i++
				opt.WorkDir = args[i]
				acpOpt.WorkDir = args[i]
			}
		case a == "--timeout":
			if i+1 < len(args) {
				i++
				if sec, err := strconv.Atoi(args[i]); err == nil {
					opt.Timeout = time.Duration(sec) * time.Second
				}
			}
		case a == "--max-iter":
			if i+1 < len(args) {
				i++
				opt.MaxIter, _ = strconv.Atoi(args[i])
				acpOpt.MaxIter = opt.MaxIter
			}
		case a == "--bind":
			if i+1 < len(args) {
				i++
				bind = args[i]
			}
		case a == "--secret":
			if i+1 < len(args) {
				i++
				secret = args[i]
			}
		case a == "--help" || a == "-h":
			fmt.Print(agentUsage())
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "unknown flag %s\n", a)
				return 2
			}
			taskParts = append(taskParts, a)
		}
	}

	// ACP stdio / serve (Phase 8)
	if mode == "stdio" {
		if acpOpt.WorkDir == "" {
			acpOpt.WorkDir, _ = os.Getwd()
		}
		// Default always-approve for IDE unless dont-ask
		if !acpOpt.DontAsk && !acpOpt.Plan {
			acpOpt.AlwaysApprove = true
		}
		srv := acp.NewServer(acpOpt)
		if err := acp.ServeStdio(srv, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "acp stdio: %v\n", err)
			return 1
		}
		return 0
	}
	if mode == "serve" {
		if acpOpt.WorkDir == "" {
			acpOpt.WorkDir, _ = os.Getwd()
		}
		if !acpOpt.DontAsk && !acpOpt.Plan {
			acpOpt.AlwaysApprove = true
		}
		if err := acp.ServeWebSocket(acp.ServeOptions{
			Bind: bind, Secret: secret, ACP: acpOpt,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "acp serve: %v\n", err)
			return 1
		}
		return 0
	}

	opt.Task = strings.Join(taskParts, " ")
	if opt.Task == "" {
		st, _ := os.Stdin.Stat()
		if (st.Mode() & os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err == nil && len(data) > 0 {
				opt.Task = strings.TrimSpace(string(data))
			}
		}
	}
	if strings.TrimSpace(opt.Task) == "" {
		fmt.Fprintln(os.Stderr, "usage: codeforge agent [flags] <task>")
		fmt.Fprint(os.Stderr, agentUsage())
		return 2
	}
	return headless.RunCLI(opt)
}

func runSessionCLI(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, sessionUsage())
		return 2
	}
	switch args[0] {
	case "list":
		list, err := session.List(30)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		for _, s := range list {
			fmt.Printf("%s  %s\n  %s\n", s.ID, s.Slug, s.Preview)
		}
		return 0
	case "export":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: codeforge session export <id> [dest.json]")
			return 2
		}
		dest := args[1] + ".json"
		if len(args) >= 3 {
			dest = args[2]
		}
		if err := session.Export(args[1], dest); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("exported", dest)
		return 0
	case "import":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: codeforge session import <file.json>")
			return 2
		}
		s, err := session.Import(args[1])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Println("imported", s.ID)
		return 0
	case "migrate":
		// Phase 9: flat v0.8 JSON → Phase 4 v2 dirs
		res, err := session.MigrateLegacy()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("migrated %d · skipped %d\n", res.Migrated, res.Skipped)
		for _, e := range res.Errors {
			fmt.Fprintln(os.Stderr, "  ⚠", e)
		}
		return 0
	case "export-all":
		dest := "codeforge-sessions"
		if len(args) >= 2 {
			dest = args[1]
		}
		n, err := session.ExportAll(dest)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		fmt.Printf("exported %d sessions → %s\n", n, dest)
		return 0
	default:
		fmt.Fprint(os.Stderr, sessionUsage())
		return 2
	}
}

func needsWizardQuick() bool {
	if os.Getenv("XAI_API_KEY") != "" || os.Getenv("GROK_API_KEY") != "" ||
		os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "" {
		return false
	}
	return true
}

func runWizard() {
	r := bufio.NewReader(os.Stdin)
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  CodeForge — First Run Setup                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("① API keys / GitHub:")
	show := func(name, env string) {
		if os.Getenv(env) != "" {
			fmt.Printf("   ✓ %s\n", name)
		} else {
			fmt.Printf("   ○ %s — set %s\n", name, env)
		}
	}
	show("Grok 4.5 (xAI)", "XAI_API_KEY")
	show("Gemini", "GEMINI_API_KEY")
	show("Claude", "ANTHROPIC_API_KEY")
	show("OpenAI", "OPENAI_API_KEY")
	show("GitHub token", "GITHUB_TOKEN")
	fmt.Println("   Grok: https://console.x.ai/  ·  Gemini: https://aistudio.google.com/apikey")
	fmt.Print("   Enter (paste XAI_ or GEMINI key): ")
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "xai-") || strings.HasPrefix(line, "xai_") {
		_ = os.Setenv("XAI_API_KEY", line)
		fmt.Println("   ✓ XAI_API_KEY set for this session (Grok 4.5)")
	} else if strings.HasPrefix(line, "AIza") || len(line) > 20 {
		_ = os.Setenv("GEMINI_API_KEY", line)
		fmt.Println("   ✓ GEMINI_API_KEY set for this session")
	}
	fmt.Println()
	fmt.Println("② Headless CI mode:  codeforge agent --json \"fix tests\"")
	fmt.Println("   Plugins dir:      ~/.codeforge/plugins/")
	fmt.Println("   Sessions sync:    CODEFORGE_SESSIONS_DIR=/shared/path")
	fmt.Print("   Enter to continue…")
	_, _ = r.ReadString('\n')
}

func printBanner() {
	fmt.Printf(`
╔══════════════════════════════════════════════════════════════╗
║   CodeForge TUI v%s  |  by %s  |  %s                 ║
║   Grok-parity TUI · ACP · Plan · Sessions · Permissions      ║
╚══════════════════════════════════════════════════════════════╝
`, ProjectVersion, ProjectAuthor, ProjectYear)
}

func printUsage() {
	fmt.Printf(`CodeForge TUI v%s — Terminal AI Coding Companion

Usage:
  codeforge [workdir] [flags]           Interactive TUI
  codeforge agent [flags] <task>        Headless agent (CI/scripts)
  codeforge agent stdio                 ACP JSON-RPC for IDEs
  codeforge agent serve                 ACP WebSocket server
  codeforge session <cmd>               Export / import sessions
  codeforge version

TUI flags:
  --no-motion       Disable animations
  --minimal         No chrome; terminal-native 16 colors
  --compact         Tighter padding (same as /compact-mode)
  --skip-wizard     Skip first-run setup
  --sandbox MODE    OS sandbox: off|workspace|read-only|strict|devbox
  -h, --help        Help
  -v, --version     Version

%s
%s
Env:
  XAI_API_KEY / GROK_API_KEY   Grok 4.5 (preferred when set)
  GEMINI_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY
  GITHUB_TOKEN / GH_TOKEN
  BRAVE_API_KEY                optional richer web_search
  CODEFORGE_SANDBOX / GROK_SANDBOX   sandbox profile (see docs/SANDBOX.md)
  CODEFORGE_REASONING      on|off|low|medium|high (native thinking tokens)
  CODEFORGE_PAGER / GROK_PAGER   path to pager.toml (see docs/PAGER.md)
  CODEFORGE_SESSIONS_DIR   shared session storage (SSH/sync)
  CODEFORGE_PLUGIN_DIR     extra plugins path
  CODEFORGE_TELEMETRY=1    opt-in local telemetry
  CODEFORGE_THEME          groknight|grokday|tokyonight|rosepine|oscura|auto
  CODEFORGE_AUTO_DARK / CODEFORGE_AUTO_LIGHT
  CODEFORGE_COMPACT=1, CODEFORGE_MINIMAL=1, CODEFORGE_NO_MOTION
  CODEFORGE_SSH_TUNE=1     auto compact+no-motion over SSH
  CODEFORGE_COLOR          true|256|16|none (force quantize)
  NO_COLOR                 monochrome (a11y)
  CODEFORGE_PLAIN_MD       skip glamour

`, ProjectVersion, agentUsage(), sessionUsage())
}

func agentUsage() string {
	return `Agent:
  codeforge agent [flags] <task>           One-shot headless run
  codeforge agent [flags] stdio            ACP JSON-RPC over stdin/stdout (IDE)
  codeforge agent [flags] serve            ACP over WebSocket

  codeforge agent --json "run go test and fix failures"
  codeforge agent --model gemini-2.5-flash --always-approve stdio
  codeforge agent serve --bind 127.0.0.1:2419 --secret tok

Flags:
  --json, -j       Machine-readable JSON result (exit 1 on failure)
  --plan           Design/plan permission mode + staged writes
  --act            Apply writes immediately (default for one-shot)
  --always-approve, --yolo  Bypass ask (deny rules still apply)
  --dont-ask       Deny anything that would prompt (CI lockdown)
  --sandbox MODE   OS sandbox: off|workspace|read-only|strict|devbox
  --model, -m      Model id for current provider
  --workdir, -C    Project directory
  --timeout SEC    One-shot timeout (default 600)
  --max-iter N     Agent iterations (default 12)
  --quiet, -q      Less human chatter
  --bind ADDR      serve bind (default 127.0.0.1:2419)
  --secret TOKEN   serve auth (or CODEFORGE_AGENT_SECRET)

ACP methods: initialize, session/new, session/load, session/prompt, session/cancel
See docs/ACP.md · docs/SANDBOX.md
`
}

func sessionUsage() string {
	return `Sessions:
  codeforge session list
  codeforge session export <id> [file.json]
  codeforge session import <file.json>
  codeforge session export-all [dir]
  codeforge session migrate              # v0.8 flat JSON → v2 layout

  Shared across machines:
    export CODEFORGE_SESSIONS_DIR=/path/to/sync/sessions
  See docs/SESSION_MIGRATION.md
`
}

