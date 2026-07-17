package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	DefaultProvider string              `mapstructure:"default_provider"`
	Providers       map[string]Provider `mapstructure:"providers"`
	Theme           string              `mapstructure:"theme"`
	DiffMode        string              `mapstructure:"diff_mode"` // side-by-side | unified
	NoMotion        bool                `mapstructure:"no_motion"`
	Git             GitConfig           `mapstructure:"git"`
	Permissions     PermissionsConfig   `mapstructure:"permissions"`
	Workspace       WorkspaceConfig     `mapstructure:"workspace"`
	Budget          BudgetConfig        `mapstructure:"budget"`
	MCP             MCPConfig           `mapstructure:"mcp"`
	Plugins         PluginsConfig       `mapstructure:"plugins"`
	Telemetry       TelemetryConfig     `mapstructure:"telemetry"`
	UI              UIConfig            `mapstructure:"ui"`
	Session         SessionConfig       `mapstructure:"session"`
	// Sandbox is Grok-compatible OS shell sandbox (Phase G4).
	Sandbox SandboxConfig `mapstructure:"sandbox"`
	// Skills is Grok-compatible SKILL.md packages (Phase G5).
	Skills SkillsConfig `mapstructure:"skills"`
	// Subagents configures personas (Phase G6).
	Subagents SubagentsConfig `mapstructure:"subagents"`
	// Secrets controls API key persistence (Q3.2 / Q3.3). Prefer env over disk.
	Secrets SecretsConfig `mapstructure:"secrets"`
}

// SubagentsConfig holds persona overlays for spawn_subagent.
type SubagentsConfig struct {
	// Personas map name → persona (overrides files).
	Personas map[string]SubagentPersona `mapstructure:"personas"`
	// ExtraDirs additional persona directories.
	ExtraDirs []string `mapstructure:"extra_dirs"`
}

// SubagentPersona is config-file shape for a persona.
type SubagentPersona struct {
	Description      string `mapstructure:"description"`
	Instructions     string `mapstructure:"instructions"`
	InstructionsFile string `mapstructure:"instructions_file"`
	Model            string `mapstructure:"model"`
	DefaultIsolation string `mapstructure:"default_isolation"`
}

// SandboxConfig selects a profile: off | workspace | read-only | strict | devbox.
type SandboxConfig struct {
	// Profile default when --sandbox / CODEFORGE_SANDBOX not set.
	Profile string `mapstructure:"profile"`
	// Deny extra paths/globs blocked for read+write (soft always; bwrap when available).
	Deny []string `mapstructure:"deny"`
}

// SkillsConfig discovers reusable SKILL.md packages (Grok-compatible).
type SkillsConfig struct {
	// Paths additional skill roots (files or directories).
	Paths []string `mapstructure:"paths"`
	// Ignore path prefixes to skip entirely.
	Ignore []string `mapstructure:"ignore"`
	// Disabled skill names (listed but not injected / not invocable).
	Disabled []string `mapstructure:"disabled"`
	// CompatClaude scan .claude/skills (default true).
	CompatClaude *bool `mapstructure:"compat_claude"`
	// CompatCursor scan .cursor/skills (default true).
	CompatCursor *bool `mapstructure:"compat_cursor"`
}

// SessionConfig controls session lifecycle (Phase 4).
type SessionConfig struct {
	// AutoCompactPct triggers /compact when tokens reach this fraction of max context (0–1).
	AutoCompactPct float64 `mapstructure:"auto_compact_pct"`
}

// UIConfig matches Grok [ui] knobs used by CodeForge (+ pager.toml [ui]).
type UIConfig struct {
	// VimMode enables j/k/h/l/g/G single-letter scrollback bindings (Grok vim_mode).
	VimMode     bool `mapstructure:"vim_mode"`
	CompactMode bool `mapstructure:"compact_mode"`
	// Theme overrides top-level theme when set (Grok [ui].theme).
	Theme string `mapstructure:"theme"`
	// AutoDarkTheme / AutoLightTheme map system appearance when theme=auto.
	AutoDarkTheme  string `mapstructure:"auto_dark_theme"`
	AutoLightTheme string `mapstructure:"auto_light_theme"`
	// SimpleMode: true = readline prompt (default), false = experimental vim prompt.
	SimpleMode *bool `mapstructure:"simple_mode"`
	// ShowThinkingBlocks controls thinking/reasoning blocks in scrollback.
	ShowThinkingBlocks *bool `mapstructure:"show_thinking_blocks"`
	// MaxThoughtsWidth caps reasoning column width.
	MaxThoughtsWidth *int `mapstructure:"max_thoughts_width"`
	// GroupToolVerbs folds consecutive read/search tool rows.
	GroupToolVerbs *bool `mapstructure:"group_tool_verbs"`
	// ScreenMode: minimal | fullscreen
	ScreenMode string `mapstructure:"screen_mode"`
	// Scroll knobs (also in pager.toml)
	ScrollSpeed  *int   `mapstructure:"scroll_speed"`
	ScrollMode   string `mapstructure:"scroll_mode"`
	ScrollLines  *int   `mapstructure:"scroll_lines"`
	InvertScroll *bool  `mapstructure:"invert_scroll"`
	// DefaultSelectedPermission: always_allow_all_sessions | allow_command_always | allow_once | reject
	DefaultSelectedPermission string `mapstructure:"default_selected_permission"`
	RememberToolApprovals     *bool  `mapstructure:"remember_tool_approvals"`
}

// PluginsConfig lists extra plugin search directories.
type PluginsConfig struct {
	Dirs []string `mapstructure:"dirs"`
}

// TelemetryConfig is privacy-first opt-in analytics (default off).
type TelemetryConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Endpoint  string `mapstructure:"endpoint"`
	LocalOnly bool   `mapstructure:"local_only"`
}

// BudgetConfig limits spend and can block further agent calls.
type BudgetConfig struct {
	// MaxCostUSD hard-stops agent/chat submits when totalCost exceeds this (0 = unlimited)
	MaxCostUSD float64 `mapstructure:"max_cost_usd"`
	// WarnAtUSD shows a toast when cost crosses this (0 = 50% of max if max set)
	WarnAtUSD float64 `mapstructure:"warn_at_usd"`
}

// MCPConfig lists stdio MCP servers to attach at startup.
type MCPConfig struct {
	Servers []MCPServer `mapstructure:"servers"`
}

// MCPServer is one MCP stdio server entry.
type MCPServer struct {
	Name    string            `mapstructure:"name"`
	Command string            `mapstructure:"command"`
	Args    []string          `mapstructure:"args"`
	Env     map[string]string `mapstructure:"env"`
}

// WorkspaceConfig enables multi-root monorepo support.
type WorkspaceConfig struct {
	// ExtraRoots are additional project roots (relative to primary or absolute).
	ExtraRoots []string `mapstructure:"extra_roots"`
	// IgnoreDirs overrides default directory ignore list when non-empty.
	IgnoreDirs []string `mapstructure:"ignore_dirs"`
}

type Provider struct {
	Enabled      bool                 `mapstructure:"enabled"`
	Type         string               `mapstructure:"type"`
	APIKey       string               `mapstructure:"api_key"`
	Endpoint     string               `mapstructure:"endpoint"`
	DefaultModel string               `mapstructure:"default_model"`
	Capabilities ProviderCapabilities `mapstructure:"capabilities"`
}

type ProviderCapabilities struct {
	Streaming  bool `mapstructure:"streaming"`
	ToolUse    bool `mapstructure:"tool_use"`
	Vision     bool `mapstructure:"vision"`
	MaxContext int  `mapstructure:"max_context"`
}

type GitConfig struct {
	AutoCommit   bool   `mapstructure:"auto_commit"`
	CommitStyle  string `mapstructure:"commit_style"`
	BranchPrefix string `mapstructure:"branch_prefix"`
}

type PermissionsConfig struct {
	RequireConfirmWrite bool `mapstructure:"require_confirm_write"`
	RequireConfirmShell bool `mapstructure:"require_confirm_shell"`
	RequireConfirmPush  bool `mapstructure:"require_confirm_push"`
	// Mode: default | plan | always_approve | dont_ask (Phase 6)
	Mode string `mapstructure:"mode"`
	// Rules is the allow/deny/ask list.
	Rules []PermissionRule `mapstructure:"rules"`
}

// PermissionRule is one allow/deny/ask entry.
type PermissionRule struct {
	Tool    string `mapstructure:"tool" yaml:"tool"`
	Pattern string `mapstructure:"pattern" yaml:"pattern"`
	Effect  string `mapstructure:"effect" yaml:"effect"` // deny | ask | allow
}

func Default() *Config {
	return &Config{
		DefaultProvider: "grok",
		Theme:           "groknight",
		DiffMode:        "unified",
		NoMotion:        false,
		Providers: map[string]Provider{
			"grok": {
				Enabled:      true,
				Type:         "xai",
				APIKey:       "",
				DefaultModel: "grok-4.5",
				Capabilities: ProviderCapabilities{
					Streaming:  true,
					ToolUse:    true,
					Vision:     true,
					MaxContext: 500000,
				},
			},
			"gemini": {
				Enabled:      true,
				Type:         "google",
				APIKey:       "",
				DefaultModel: "gemini-2.5-flash",
				Capabilities: ProviderCapabilities{
					Streaming:  true,
					ToolUse:    true,
					Vision:     true,
					MaxContext: 1048576,
				},
			},
			"claude": {
				Enabled:      true,
				Type:         "anthropic",
				APIKey:       "",
				DefaultModel: "claude-sonnet-4-20250514",
				Capabilities: ProviderCapabilities{
					Streaming:  true,
					ToolUse:    true,
					Vision:     true,
					MaxContext: 200000,
				},
			},
			"openai": {
				Enabled:      true,
				Type:         "openai",
				APIKey:       "",
				DefaultModel: "gpt-4o-mini",
				Capabilities: ProviderCapabilities{
					Streaming:  true,
					ToolUse:    true,
					Vision:     true,
					MaxContext: 128000,
				},
			},
		},
		Git: GitConfig{
			AutoCommit:   true,
			CommitStyle:  "conventional",
			BranchPrefix: "ai/",
		},
		Permissions: PermissionsConfig{
			RequireConfirmWrite: true,
			RequireConfirmShell: true,
			RequireConfirmPush:  true,
			Mode:                "default",
			Rules:               nil,
		},
		Workspace: WorkspaceConfig{
			ExtraRoots: nil,
			IgnoreDirs: nil,
		},
		Budget: BudgetConfig{
			MaxCostUSD: 0,
			WarnAtUSD:  0,
		},
		MCP:     MCPConfig{Servers: nil},
		Plugins: PluginsConfig{Dirs: nil},
		Telemetry: TelemetryConfig{
			Enabled:   false,
			LocalOnly: true,
		},
		UI: UIConfig{
			VimMode:        false,
			CompactMode:    false,
			Theme:          "",
			AutoDarkTheme:  "groknight",
			AutoLightTheme: "grokday",
		},
		Session: SessionConfig{
			AutoCompactPct: 0.85,
		},
		Sandbox: SandboxConfig{
			Profile: "off",
			Deny:    nil,
		},
		Skills: SkillsConfig{
			Paths:    nil,
			Ignore:   nil,
			Disabled: nil,
		},
		Subagents: SubagentsConfig{
			Personas:  nil,
			ExtraDirs: nil,
		},
		Secrets: SecretsConfig{
			Backend: SecretsAuto,
		},
	}
}

// SkillsCompatClaude returns whether Claude skill dirs are scanned (default true).
func (c *Config) SkillsCompatClaude() bool {
	if c == nil || c.Skills.CompatClaude == nil {
		return true
	}
	return *c.Skills.CompatClaude
}

// SkillsCompatCursor returns whether Cursor skill dirs are scanned (default true).
func (c *Config) SkillsCompatCursor() bool {
	if c == nil || c.Skills.CompatCursor == nil {
		return true
	}
	return *c.Skills.CompatCursor
}

// ConfigDir returns the directory for config.yaml (Q3 tests: CODEFORGE_CONFIG_DIR).
// Precedence: CODEFORGE_CONFIG_DIR → XDG_CONFIG_HOME/codeforge → ~/.config/codeforge.
func ConfigDir() (string, error) {
	if d := strings.TrimSpace(os.Getenv("CODEFORGE_CONFIG_DIR")); d != "" {
		return d, nil
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "codeforge"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "codeforge"), nil
}

// LoadRaw loads config without secret resolution or validation (tests / repair).
func LoadRaw() (*Config, error) {
	return loadUnchecked()
}

func loadUnchecked() (*Config, error) {
	cfg := Default()
	v := viper.New()
	v.SetConfigType("yaml")
	v.SetDefault("default_provider", cfg.DefaultProvider)
	v.SetDefault("theme", cfg.Theme)
	v.SetDefault("git.auto_commit", cfg.Git.AutoCommit)
	v.SetDefault("git.commit_style", cfg.Git.CommitStyle)
	v.SetDefault("git.branch_prefix", cfg.Git.BranchPrefix)
	v.SetDefault("permissions.require_confirm_write", cfg.Permissions.RequireConfirmWrite)
	v.SetDefault("permissions.require_confirm_shell", cfg.Permissions.RequireConfirmShell)
	v.SetDefault("permissions.require_confirm_push", cfg.Permissions.RequireConfirmPush)

	cfgDir, _ := ConfigDir()
	v.AddConfigPath(cfgDir)
	v.AddConfigPath(".")
	v.SetConfigName("config")

	_ = v.BindEnv("providers.claude.api_key", "ANTHROPIC_API_KEY")
	_ = v.BindEnv("providers.gemini.api_key", "GEMINI_API_KEY")
	_ = v.BindEnv("providers.openai.api_key", "OPENAI_API_KEY")
	_ = v.BindEnv("providers.grok.api_key", "XAI_API_KEY")
	_ = v.BindEnv("providers.xai.api_key", "XAI_API_KEY")
	_ = v.BindEnv("sandbox.profile", "CODEFORGE_SANDBOX")
	_ = v.BindEnv("theme", "CODEFORGE_THEME")
	_ = v.BindEnv("secrets.backend", "CODEFORGE_SECRETS_BACKEND")

	if err := v.ReadInConfig(); err != nil {
		// file not found OK
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}

	if cfg.Providers == nil {
		cfg.Providers = make(map[string]Provider)
	}
	return cfg, nil
}

// Load reads config, resolves secrets (prefer env), validates schema (Q3.5).
func Load() (*Config, error) {
	cfg, err := loadUnchecked()
	if err != nil {
		return nil, err
	}
	// Prefer env → keystore → config.yaml (Q3.2). Never require config keys.
	fillKey := func(name string) {
		p, ok := cfg.Providers[name]
		if !ok {
			p = Provider{Enabled: true, Type: defaultProviderType(name)}
		}
		if resolved := ResolveAPIKey(name, p.APIKey); resolved != "" {
			p.APIKey = resolved
		}
		cfg.Providers[name] = p
	}
	fillKey("claude")
	fillKey("gemini")
	fillKey("openai")
	fillKey("grok")
	fillKey("xai")

	_ = EnsureConfigPermissions()

	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// SaveProviderKey stores an API key using secrets.backend and updates model in YAML.
// Prefer env when already set (no disk write for the secret). Model/default still update.
//
// Storage policy (Q3.2 / Q3.3):
//  1. If env already has a key for this provider → do not write secret; update model only
//  2. secrets.backend=env_only → error (export env instead)
//  3. auto/keyring/file → OS keyring and/or keys/<provider>.key (0600), not config.yaml
//  4. model + default_provider always updated non-destructively in config.yaml
func SaveProviderKey(name, apiKey, model string) error {
	name = normalizeProviderName(name)
	if name == "" || strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("provider and api key required")
	}
	cfg, _ := loadUnchecked()
	if cfg == nil {
		cfg = Default()
	}
	backend := effectiveBackend(cfg.Secrets.Backend)

	// Persist secret outside YAML when possible (prefer env → skip disk).
	if err := StoreAPIKey(name, apiKey, backend, true); err != nil {
		return err
	}

	// Metadata only in YAML — never re-inject env keys; avoid plaintext when file/keyring used.
	return setProviderYAML(name, "", model, true)
}

// SaveDefaultProvider sets default_provider only — never rewrites API keys (Q3.1).
func SaveDefaultProvider(name string) error {
	return setDefaultProviderYAML(name)
}

// SaveExample writes a starter config.yaml if missing (mode 0600).
func SaveExample() error {
	cfgDir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cfgDir, DirModeConfig); err != nil {
		return err
	}
	examplePath := filepath.Join(cfgDir, "config.yaml")
	if _, err := os.Stat(examplePath); err == nil {
		_ = os.Chmod(examplePath, FileModeConfig)
		return nil
	}
	content := `# CodeForge TUI Configuration
# Keys: prefer env (XAI_API_KEY, GEMINI_API_KEY, …). See docs/SECRETS.md
default_provider: grok
theme: groknight

# secrets:
#   backend: auto      # auto | file | keyring | env_only

providers:
  grok:
    enabled: true
    type: xai
    api_key: ""          # optional — prefer XAI_API_KEY / GROK_API_KEY
    default_model: grok-4.5
  gemini:
    enabled: true
    type: google
    api_key: ""          # optional — prefer GEMINI_API_KEY
    default_model: gemini-2.5-flash
  claude:
    enabled: true
    type: anthropic
    api_key: ""          # optional — prefer ANTHROPIC_API_KEY
    default_model: claude-sonnet-4-20250514

# sandbox:
#   profile: workspace   # off | workspace | read-only | strict | devbox

# ui:
#   vim_mode: false
#   compact_mode: false
#   show_thinking_blocks: true
#   scroll_speed: 50

git:
  auto_commit: true
  commit_style: conventional
  branch_prefix: ai/

permissions:
  mode: default   # default | plan | always_approve | dont_ask
  require_confirm_write: true
  require_confirm_shell: true
  require_confirm_push: true
  # rules:
  #   - tool: run_command
  #     pattern: "rm -rf *"
  #     effect: deny
  #   - tool: run_command
  #     pattern: "go test *"
  #     effect: allow
`
	return writeFileAtomic(examplePath, []byte(content), FileModeConfig)
}
