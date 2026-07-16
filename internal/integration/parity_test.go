// Package integration runs cross-package smoke for Grok parity v1.0.
package integration

import (
	"testing"

	"github.com/codeforge/tui/internal/acp"
	"github.com/codeforge/tui/internal/config"
	"github.com/codeforge/tui/internal/permission"
	"github.com/codeforge/tui/internal/provider"
	"github.com/codeforge/tui/internal/session"
	"github.com/codeforge/tui/internal/theme"
	"github.com/codeforge/tui/internal/todos"
	"github.com/codeforge/tui/internal/tool"
	"github.com/codeforge/tui/internal/tui"
	"github.com/codeforge/tui/internal/tui/blocks"
)

func TestThemeA11yNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("CODEFORGE_THEME", "")
	t.Setenv("CODEFORGE_COLOR", "")
	t.Setenv("CODEFORGE_MINIMAL", "")
	theme.ResetColorLevelCache()
	theme.InitFromEnv()
	if theme.MotionEnabled() {
		t.Fatal("motion should be off under NO_COLOR")
	}
	// cleanup
	theme.SetMinimal(false)
	theme.SetByName("groknight")
	theme.SetMotion(true)
	theme.ResetColorLevelCache()
}

func TestPermissionsDenyIntegrated(t *testing.T) {
	eng := permission.NewEngine(t.TempDir())
	res := eng.Evaluate("run_command", `{"command":"rm -rf /tmp/x"}`)
	if res.Decision != permission.DecisionDeny {
		t.Fatal(res)
	}
}

func TestSessionV2RoundTrip(t *testing.T) {
	t.Setenv("CODEFORGE_SESSIONS_DIR", t.TempDir())
	s := session.New("gemini", "flash", t.TempDir())
	s.Messages = []provider.Message{{Role: provider.RoleUser, Content: "integration"}}
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := session.Load(s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Format != "v2" || len(loaded.Messages) != 1 {
		t.Fatal(loaded)
	}
}

func TestToolsRegisterPhaseStack(t *testing.T) {
	reg := tool.NewRegistry(t.TempDir())
	need := []string{
		"write_file", "search_replace", "run_command",
		"write_plan", "exit_plan_mode", "todo_write",
		// Grok 4.5 surface
		"web_search", "web_fetch", "grep", "run_terminal_command",
		"memory_search", "memory_write", "spawn_subagent", "ask_user_question",
		"glob_file_search", "glob", "find_files", "ask_user",
	}
	for _, n := range need {
		if _, ok := reg.Get(n); !ok {
			t.Fatalf("missing tool %s", n)
		}
	}
	if sw := reg.GetStagedWriter(); sw == nil {
		t.Fatal("staged writer")
	}
}

func TestACPServerConstruct(t *testing.T) {
	srv := acp.NewServer(acp.Options{Version: "1.0.0", WorkDir: t.TempDir(), AlwaysApprove: true})
	if srv == nil {
		t.Fatal("nil")
	}
}

func TestTUIModelConstruct(t *testing.T) {
	theme.SetMotion(false)
	theme.SetMinimal(false)
	theme.SetByName("groknight")
	reg := provider.NewRegistry()
	_ = reg.Register(provider.NewGeminiProvider("k", "gemini-2.5-flash"))
	tools := tool.NewRegistry(t.TempDir())
	cfg := config.Default()
	_ = tui.New(cfg, reg, tools, nil, t.TempDir())
	todos.Global.Clear()
}

func TestBlocksViewportMany(t *testing.T) {
	s := blocks.NewStore()
	s.SetSize(80, 24)
	for i := 0; i < 500; i++ {
		s.AddSystem("x")
	}
	if s.View() == "" {
		t.Fatal("view")
	}
}

func TestConfigDefaultTheme(t *testing.T) {
	cfg := config.Default()
	if cfg.Theme != "groknight" {
		t.Fatal(cfg.Theme)
	}
}
