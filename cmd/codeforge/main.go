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

	"github.com/codeforge/tui/internal/app"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/headless"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tui"
)

const (
	ProjectName    = "CodeForge TUI"
	ProjectVersion = "0.9.6"
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
	var pathArgs []string
	for _, a := range args {
		switch a {
		case "--no-motion":
			noMotion = true
		case "--minimal":
			minimal = true
		case "--compact":
			compact = true
		case "--skip-wizard", "--yes", "-y":
			skipWizard = true
		case "--help", "-h":
			printUsage()
			return
		case "--version", "-v":
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

	rt, err := app.Bootstrap(app.Options{WorkDir: workdir})
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
	opt := headless.Options{
		JSON:    false,
		Act:     true,
		Timeout: 10 * time.Minute,
		MaxIter: 12,
	}
	var taskParts []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--json", "-j":
			opt.JSON = true
			opt.Quiet = true
		case "--plan":
			opt.Plan = true
			opt.Act = false
		case "--act":
			opt.Act = true
			opt.Plan = false
		case "--always-approve", "--yolo":
			opt.AlwaysApprove = true
			opt.Act = true
			opt.Plan = false
		case "--dont-ask":
			opt.DontAsk = true
		case "--model", "-m":
			if i+1 < len(args) {
				i++
				opt.Model = args[i]
			}
		case "--quiet", "-q":
			opt.Quiet = true
		case "--workdir", "-C":
			if i+1 < len(args) {
				i++
				opt.WorkDir = args[i]
			}
		case "--timeout":
			if i+1 < len(args) {
				i++
				if sec, err := strconv.Atoi(args[i]); err == nil {
					opt.Timeout = time.Duration(sec) * time.Second
				}
			}
		case "--max-iter":
			if i+1 < len(args) {
				i++
				opt.MaxIter, _ = strconv.Atoi(args[i])
			}
		case "--help", "-h":
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
	if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("OPENAI_API_KEY") != "" {
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
	show("Gemini", "GEMINI_API_KEY")
	show("Claude", "ANTHROPIC_API_KEY")
	show("OpenAI", "OPENAI_API_KEY")
	show("GitHub token", "GITHUB_TOKEN")
	fmt.Println("   Get Gemini: https://aistudio.google.com/apikey")
	fmt.Print("   Enter (or paste GEMINI key): ")
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "AIza") || len(line) > 20 {
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
║   Grok-style TUI · Plan/Act · Headless · Plugins · GitHub    ║
╚══════════════════════════════════════════════════════════════╝
`, ProjectVersion, ProjectAuthor, ProjectYear)
}

func printUsage() {
	fmt.Printf(`CodeForge TUI v%s — Terminal AI Coding Companion

Usage:
  codeforge [workdir] [flags]           Interactive TUI
  codeforge agent [flags] <task>        Headless agent (CI/scripts)
  codeforge session <cmd>               Export / import sessions
  codeforge version

TUI flags:
  --no-motion       Disable animations
  --minimal         No chrome; terminal-native 16 colors
  --compact         Tighter padding (same as /compact-mode)
  --skip-wizard     Skip first-run setup
  -h, --help        Help
  -v, --version     Version

%s
%s
Env:
  GEMINI_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY
  GITHUB_TOKEN / GH_TOKEN
  CODEFORGE_SESSIONS_DIR   shared session storage (SSH/sync)
  CODEFORGE_PLUGIN_DIR     extra plugins path
  CODEFORGE_TELEMETRY=1    opt-in local telemetry
  CODEFORGE_THEME          groknight|grokday|tokyonight|rosepine|oscura|auto
  CODEFORGE_AUTO_DARK / CODEFORGE_AUTO_LIGHT
  CODEFORGE_COMPACT=1, CODEFORGE_MINIMAL=1, CODEFORGE_NO_MOTION
  CODEFORGE_COLOR          true|256|16|none (force quantize)
  CODEFORGE_PLAIN_MD       skip glamour

`, ProjectVersion, agentUsage(), sessionUsage())
}

func agentUsage() string {
	return `Agent (headless):
  codeforge agent [flags] <task>
  codeforge agent --json "run go test and fix failures"
  echo "summarize README" | codeforge agent --json

  --json, -j       Machine-readable JSON result (exit 1 on failure)
  --plan           Design/plan permission mode + staged writes
  --act            Apply writes immediately (default)
  --always-approve, --yolo  Bypass ask (deny rules still apply)
  --dont-ask       Deny anything that would prompt (CI lockdown)
  --model, -m      Model id for current provider
  --workdir, -C    Project directory
  --timeout SEC    Overall timeout (default 600)
  --max-iter N     Agent iterations (default 12)
  --quiet, -q      Less human chatter
`
}

func sessionUsage() string {
	return `Sessions:
  codeforge session list
  codeforge session export <id> [file.json]
  codeforge session import <file.json>
  codeforge session export-all [dir]

  Shared across machines:
    export CODEFORGE_SESSIONS_DIR=/path/to/sync/sessions
`
}

