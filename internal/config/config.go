package config

import (
    "fmt"
    "os"
    "path/filepath"

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
}

func Default() *Config {
    return &Config{
        DefaultProvider: "gemini",
        Theme:           "aurora",
        DiffMode:        "unified",
        NoMotion:        false,
        Providers: map[string]Provider{
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
        },
    }
}

func ConfigDir() (string, error) {
    home, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    return filepath.Join(home, ".config", "codeforge"), nil
}

func Load() (*Config, error) {
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

    v.BindEnv("providers.claude.api_key", "ANTHROPIC_API_KEY")

    if err := v.ReadInConfig(); err != nil {
        // file not found OK
    }

    if err := v.Unmarshal(cfg); err != nil {
        return nil, fmt.Errorf("unmarshal: %w", err)
    }

    if cfg.Providers == nil {
        cfg.Providers = make(map[string]Provider)
    }
    if p, ok := cfg.Providers["claude"]; ok {
        if p.APIKey == "" {
            p.APIKey = os.Getenv("ANTHROPIC_API_KEY")
            cfg.Providers["claude"] = p
        }
    }
    return cfg, nil
}

func SaveExample() error {
    cfgDir, err := ConfigDir()
    if err != nil {
        return err
    }
    if err := os.MkdirAll(cfgDir, 0755); err != nil {
        return err
    }
    examplePath := filepath.Join(cfgDir, "config.yaml")
    if _, err := os.Stat(examplePath); err == nil {
        return nil
    }
    content := `# CodeForge TUI Configuration
default_provider: claude
theme: dark

providers:
  claude:
    enabled: true
    type: anthropic
    api_key: ""
    default_model: claude-sonnet-4-20250514
    capabilities:
      streaming: true
      tool_use: true
      vision: true
      max_context: 200000

git:
  auto_commit: true
  commit_style: conventional
  branch_prefix: ai/

permissions:
  require_confirm_write: true
  require_confirm_shell: true
  require_confirm_push: true
`
    return os.WriteFile(examplePath, []byte(content), 0644)
}
