// CodeForge TUI - Terminal AI Coding Companion
// Author: NanoMind (2026)
// License: Apache 2.0
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/git"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/tui"
)

const (
	ProjectName    = "CodeForge TUI"
	ProjectVersion = "0.3.0"
	ProjectAuthor  = "NanoMind"
	ProjectYear    = "2026"
	ProjectLicense = "Apache 2.0"
)

func main() {
	// Flags
	noMotion := false
	skipWizard := false
	var pathArgs []string
	for _, a := range os.Args[1:] {
		switch a {
		case "--no-motion":
			noMotion = true
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
	if noMotion {
		theme.SetMotion(false)
	}

	workdir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if len(pathArgs) > 0 {
		if abs, err := filepath.Abs(pathArgs[0]); err == nil {
			if info, err := os.Stat(abs); err == nil && info.IsDir() {
				workdir = abs
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		cfg = config.Default()
	}
	_ = config.SaveExample()

	provReg := provider.NewRegistry()
	registerProviders(provReg)

	// First-run wizard if no keys and not skipped
	if !skipWizard && needsWizard(provReg) {
		runWizard(provReg)
		// re-register after env may have been set (wizard only guides)
		provReg = provider.NewRegistry()
		registerProviders(provReg)
	}

	if _, err := provReg.Current(); err != nil {
		// last resort empty claude so TUI still opens
		_ = provReg.Register(provider.NewClaudeProvider("", "claude-sonnet-4-20250514"))
	}

	// Prefer Gemini free tier when available
	if os.Getenv("GEMINI_API_KEY") != "" {
		_ = provReg.Switch("gemini")
	} else if cfg.DefaultProvider != "" {
		_ = provReg.Switch(cfg.DefaultProvider)
	}

	toolReg := tool.NewRegistry(workdir)
	if cfg.Permissions.RequireConfirmWrite {
		if sw := toolReg.GetStagedWriter(); sw != nil {
			sw.SetMode(tool.ModePlan)
		}
	} else {
		if sw := toolReg.GetStagedWriter(); sw != nil {
			sw.SetMode(tool.ModeAct)
		}
	}

	repo, err := git.Open(workdir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: git: %v\n", err)
	}

	if cur, err := provReg.Current(); err == nil {
		if err := cur.ValidateConfig(); err != nil {
			fmt.Fprintf(os.Stderr, "\n⚠️  Provider config: %v\n", err)
			fmt.Fprintf(os.Stderr, "   Gemini free: https://aistudio.google.com/apikey\n")
			fmt.Fprintf(os.Stderr, "   export GEMINI_API_KEY=...\n\n")
		}
	}

	model := tui.New(cfg, provReg, toolReg, repo, workdir)
	printBanner()

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func registerProviders(reg *provider.Registry) {
	if k := os.Getenv("GEMINI_API_KEY"); k != "" {
		_ = reg.Register(provider.NewGeminiProvider(k, "gemini-2.5-flash"))
		fmt.Fprintf(os.Stderr, "✓ Gemini registered\n")
	}
	if k := os.Getenv("ANTHROPIC_API_KEY"); k != "" {
		_ = reg.Register(provider.NewClaudeProvider(k, "claude-sonnet-4-20250514"))
		fmt.Fprintf(os.Stderr, "✓ Claude registered\n")
	}
	if k := os.Getenv("OPENAI_API_KEY"); k != "" {
		_ = reg.Register(provider.NewOpenAIProvider(k, "gpt-4o-mini"))
		fmt.Fprintf(os.Stderr, "✓ OpenAI registered\n")
	}
	// Ollama optional — only if reachable
	ollama := provider.NewOllamaProvider("")
	if err := ollama.ValidateConfig(); err == nil {
		_ = reg.Register(ollama)
		fmt.Fprintf(os.Stderr, "✓ Ollama registered (local)\n")
	}
}

func needsWizard(reg *provider.Registry) bool {
	// no valid provider config
	cur, err := reg.Current()
	if err != nil {
		return true
	}
	return cur.ValidateConfig() != nil
}

func runWizard(reg *provider.Registry) {
	r := bufio.NewReader(os.Stdin)
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════╗")
	fmt.Println("║  CodeForge — First Run Setup                         ║")
	fmt.Println("╚══════════════════════════════════════════════════════╝")
	fmt.Println()

	// Screen 1: API keys
	fmt.Println("① API Keys terdeteksi:")
	show := func(name, env string) {
		if os.Getenv(env) != "" {
			fmt.Printf("   ✓ %s (%s)\n", name, env)
		} else {
			fmt.Printf("   ○ %s — set %s\n", name, env)
		}
	}
	show("Gemini (gratis)", "GEMINI_API_KEY")
	show("Claude", "ANTHROPIC_API_KEY")
	show("OpenAI", "OPENAI_API_KEY")
	fmt.Println("   ○ Ollama lokal — jalankan `ollama serve`")
	fmt.Println()
	fmt.Println("   Get Gemini free key: https://aistudio.google.com/apikey")
	fmt.Print("   Enter untuk lanjut (atau ketik key GEMINI sekarang): ")
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "AIza") || len(line) > 20 {
		_ = os.Setenv("GEMINI_API_KEY", line)
		fmt.Println("   ✓ GEMINI_API_KEY di-set untuk sesi ini")
	}

	// Screen 2: provider pick (informational)
	fmt.Println()
	fmt.Println("② Provider default: Gemini jika key ada, else Claude/OpenAI/Ollama")
	fmt.Println("   Ganti nanti dengan /provider dan /model")
	fmt.Print("   Enter…")
	_, _ = r.ReadString('\n')

	// Screen 3: keybindings
	fmt.Println()
	fmt.Println("③ Keybinding penting:")
	fmt.Println("   i          chat          Ctrl+K   command palette")
	fmt.Println("   /act task  agent mode    Shift+P  Plan ↔ Act")
	fmt.Println("   @          mention file  ?        help")
	fmt.Println("   q          quit")
	fmt.Println()
	fmt.Print("   Enter untuk membuka CodeForge…")
	_, _ = r.ReadString('\n')
}

func printBanner() {
	fmt.Printf(`
╔══════════════════════════════════════════════════════════════╗
║   CodeForge TUI v%s  |  by %s  |  %s                 ║
║   Terminal Glass · Plan/Act · Multi-Provider                 ║
║   Gemini · Claude · OpenAI · Ollama                          ║
╚══════════════════════════════════════════════════════════════╝
`, ProjectVersion, ProjectAuthor, ProjectYear)
}

func printUsage() {
	fmt.Printf(`CodeForge TUI v%s — Terminal AI Coding Companion

Usage:
  codeforge [workdir] [flags]

Flags:
  --no-motion      Disable animations (slow SSH / Termux)
  --skip-wizard    Skip first-run setup
  -h, --help       Show help
  -v, --version    Show version

Env:
  GEMINI_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY
  OPENAI_BASE_URL, OLLAMA_HOST, OLLAMA_MODEL
  CODEFORGE_THEME=aurora|light
  CODEFORGE_NO_MOTION=1
  NERD_FONT=1

`, ProjectVersion)
}
