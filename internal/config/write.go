package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// FileModeConfig is the permission for config.yaml and secret key files (Q3.2).
const FileModeConfig = 0o600

// DirModeConfig is the permission for the config directory.
const DirModeConfig = 0o700

// mergeWriteYAML loads path as a generic YAML map (if present), applies mutate,
// and writes atomically with mode 0600. Unknown top-level keys and nested maps
// not touched by mutate are preserved (non-destructive; Q3.1).
//
// Comments and key order may change — YAML round-trip limitation — but no
// structural keys are dropped solely because they are unknown to Config.
func mergeWriteYAML(path string, mutate func(root map[string]any) error) error {
	if path == "" {
		return fmt.Errorf("config path required")
	}
	if err := os.MkdirAll(filepath.Dir(path), DirModeConfig); err != nil {
		return err
	}

	root := map[string]any{}
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := yaml.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parse existing config: %w", err)
		}
		if root == nil {
			root = map[string]any{}
		}
	}

	if err := mutate(root); err != nil {
		return err
	}

	out, err := yaml.Marshal(root)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	return writeFileAtomic(path, out, FileModeConfig)
}

// writeFileAtomic writes data to path via temp+rename and enforces mode.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, DirModeConfig); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	// Re-apply mode in case rename preserved old permissions on some FS.
	_ = os.Chmod(path, mode)
	return nil
}

// configPath returns ~/.config/codeforge/config.yaml (or CODEFORGE_CONFIG_DIR).
func configPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// writeConfig merges structured fields from cfg into the on-disk YAML without
// wiping unknown keys. Prefer targeted helpers (SaveDefaultProvider,
// SaveProviderKey) over dumping a full Load()-filled Config that may contain
// env-sourced secrets.
func writeConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	return mergeWriteYAML(path, func(root map[string]any) error {
		if cfg.DefaultProvider != "" {
			root["default_provider"] = cfg.DefaultProvider
		}
		if cfg.Theme != "" {
			root["theme"] = cfg.Theme
		}
		// Only write provider fields that are intentionally set on cfg —
		// callers that loaded via Load() may have env keys; use
		// SaveProviderKey / setProviderYAML instead for secrets.
		if len(cfg.Providers) > 0 {
			providers := mapAsMap(root["providers"])
			for name, p := range cfg.Providers {
				entry := mapAsMap(providers[name])
				entry["enabled"] = p.Enabled
				if p.Type != "" {
					entry["type"] = p.Type
				}
				if p.APIKey != "" {
					entry["api_key"] = p.APIKey
				}
				if p.DefaultModel != "" {
					entry["default_model"] = p.DefaultModel
				}
				if p.Endpoint != "" {
					entry["endpoint"] = p.Endpoint
				}
				providers[name] = entry
			}
			root["providers"] = providers
		}
		if cfg.Secrets.Backend != "" {
			sec := mapAsMap(root["secrets"])
			sec["backend"] = string(cfg.Secrets.Backend)
			root["secrets"] = sec
		}
		return nil
	})
}

// setProviderYAML updates only one provider entry (and optional default_provider).
func setProviderYAML(name, apiKey, model string, setDefault bool) error {
	name = normalizeProviderName(name)
	if name == "" {
		return fmt.Errorf("provider name required")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	return mergeWriteYAML(path, func(root map[string]any) error {
		if setDefault {
			root["default_provider"] = name
		}
		providers := mapAsMap(root["providers"])
		entry := mapAsMap(providers[name])
		entry["enabled"] = true
		if typ := defaultProviderType(name); typ != "" {
			if _, ok := entry["type"]; !ok || entry["type"] == "" || entry["type"] == nil {
				entry["type"] = typ
			}
		}
		if strings.TrimSpace(apiKey) != "" {
			entry["api_key"] = strings.TrimSpace(apiKey)
		}
		if strings.TrimSpace(model) != "" {
			entry["default_model"] = strings.TrimSpace(model)
		}
		providers[name] = entry
		root["providers"] = providers
		return nil
	})
}

// setDefaultProviderYAML only flips default_provider — never rewrites keys.
func setDefaultProviderYAML(name string) error {
	name = normalizeProviderName(name)
	if name == "" {
		return fmt.Errorf("provider name required")
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	return mergeWriteYAML(path, func(root map[string]any) error {
		root["default_provider"] = name
		return nil
	})
}

func mapAsMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	switch m := v.(type) {
	case map[string]any:
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprint(k)] = val
		}
		return out
	default:
		return map[string]any{}
	}
}

func normalizeProviderName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "xai" {
		return "grok"
	}
	return name
}

func defaultProviderType(name string) string {
	switch name {
	case "grok":
		return "xai"
	case "claude":
		return "anthropic"
	case "gemini":
		return "google"
	default:
		return name
	}
}

// EnsureConfigPermissions chmods config.yaml to 0600 if it exists (best-effort).
func EnsureConfigPermissions() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil // missing is fine
	}
	if st.Mode().Perm()&0o077 != 0 {
		return os.Chmod(path, FileModeConfig)
	}
	return nil
}
