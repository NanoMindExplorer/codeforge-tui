package config

import (
	"fmt"
	"strings"
)

// ValidationError lists one or more config problems (Q3.5).
type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "invalid config"
	}
	return "invalid config:\n  - " + strings.Join(e.Issues, "\n  - ")
}

// ValidSandboxProfiles are accepted sandbox.profile values.
var ValidSandboxProfiles = []string{"off", "workspace", "read-only", "strict", "devbox"}

// ValidPermissionModes are accepted permissions.mode values.
var ValidPermissionModes = []string{"default", "plan", "always_approve", "dont_ask"}

// ValidDiffModes are accepted diff_mode values.
var ValidDiffModes = []string{"unified", "side-by-side"}

// ValidSecretsBackends are accepted secrets.backend values.
var ValidSecretsBackends = []string{"", "auto", "file", "keyring", "env_only"}

// Validate checks cfg for known-invalid values. Empty/zero fields that Load
// fills with defaults are accepted. Returns nil when cfg is usable.
func Validate(cfg *Config) error {
	if cfg == nil {
		return &ValidationError{Issues: []string{"config is nil"}}
	}
	var issues []string

	if p := strings.TrimSpace(strings.ToLower(cfg.Sandbox.Profile)); p != "" {
		if !oneOf(p, ValidSandboxProfiles) {
			issues = append(issues, fmt.Sprintf(
				"sandbox.profile %q is invalid (want: %s)",
				cfg.Sandbox.Profile, strings.Join(ValidSandboxProfiles, " | ")))
		}
	}

	if m := strings.TrimSpace(strings.ToLower(cfg.Permissions.Mode)); m != "" {
		if !oneOf(m, ValidPermissionModes) {
			issues = append(issues, fmt.Sprintf(
				"permissions.mode %q is invalid (want: %s)",
				cfg.Permissions.Mode, strings.Join(ValidPermissionModes, " | ")))
		}
	}

	if d := strings.TrimSpace(strings.ToLower(cfg.DiffMode)); d != "" {
		if !oneOf(d, ValidDiffModes) {
			issues = append(issues, fmt.Sprintf(
				"diff_mode %q is invalid (want: %s)",
				cfg.DiffMode, strings.Join(ValidDiffModes, " | ")))
		}
	}

	if cfg.Session.AutoCompactPct < 0 || cfg.Session.AutoCompactPct > 1 {
		issues = append(issues, fmt.Sprintf(
			"session.auto_compact_pct %.3f must be between 0 and 1", cfg.Session.AutoCompactPct))
	}

	if cfg.Budget.MaxCostUSD < 0 {
		issues = append(issues, "budget.max_cost_usd must be >= 0")
	}
	if cfg.Budget.WarnAtUSD < 0 {
		issues = append(issues, "budget.warn_at_usd must be >= 0")
	}
	if cfg.Budget.MaxCostUSD > 0 && cfg.Budget.WarnAtUSD > cfg.Budget.MaxCostUSD {
		issues = append(issues, "budget.warn_at_usd cannot exceed max_cost_usd")
	}

	if b := strings.TrimSpace(strings.ToLower(string(cfg.Secrets.Backend))); b != "" {
		if !oneOf(b, ValidSecretsBackends) {
			issues = append(issues, fmt.Sprintf(
				"secrets.backend %q is invalid (want: auto | file | keyring | env_only)",
				cfg.Secrets.Backend))
		}
	}

	for name, p := range cfg.Providers {
		if p.Type != "" {
			// soft: only flag obviously empty names
			_ = name
		}
	}

	if len(issues) == 0 {
		return nil
	}
	return &ValidationError{Issues: issues}
}

func oneOf(v string, opts []string) bool {
	for _, o := range opts {
		if o == v {
			return true
		}
	}
	return false
}
