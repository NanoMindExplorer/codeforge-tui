// Package onboarding tracks first-run setup and multi-provider key helpers.
package onboarding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// State is persisted at ~/.codeforge/onboarding.json.
type State struct {
	Completed bool `json:"completed"`
	Skipped   bool `json:"skipped"`
	// WelcomeShown is true after the TUI welcome banner was displayed once.
	WelcomeShown bool      `json:"welcome_shown,omitempty"`
	Provider     string    `json:"provider,omitempty"`
	Model        string    `json:"model,omitempty"`
	UpdatedAt    time.Time `json:"updated_at,omitempty"`
}

// Dir returns ~/.codeforge.
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codeforge"), nil
}

// Path is the onboarding state file.
func Path() (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "onboarding.json"), nil
}

// Load reads state; missing file → zero value (not completed).
func Load() (State, error) {
	p, err := Path()
	if err != nil {
		return State{}, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, nil
		}
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	return s, nil
}

// Save writes state (creates ~/.codeforge if needed).
func Save(s State) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	s.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o644)
}

// MarkCompleted records a successful setup / preference.
func MarkCompleted(provider, model string) error {
	st, _ := Load()
	st.Completed = true
	st.Skipped = false
	st.Provider = normalizeName(provider)
	if model != "" {
		st.Model = model
	}
	return Save(st)
}

// MarkSkipped records user skip (--skip-wizard / decline).
func MarkSkipped() error {
	st, _ := Load()
	st.Skipped = true
	return Save(st)
}

// MarkWelcomeShown records that the TUI welcome was shown.
func MarkWelcomeShown() error {
	st, _ := Load()
	st.WelcomeShown = true
	return Save(st)
}

// ShouldShowWelcome is true until the user has seen the welcome once
// (or always when unhealthy — keep reminding until /setup).
func ShouldShowWelcome(healthy bool) bool {
	if !healthy {
		return true
	}
	st, err := Load()
	if err != nil {
		return true
	}
	return !st.WelcomeShown
}

// NeedsWizard reports whether the CLI first-run wizard should run.
//
// Runs when (unless skipFlag / Skipped):
//   - no API keys at all, or
//   - multiple cloud keys and user never completed onboarding preference, or
//   - not completed and never skipped (single-key confirm path)
func NeedsWizard(skipFlag bool) bool {
	if skipFlag {
		return false
	}
	st, err := Load()
	if err == nil && st.Skipped {
		return false
	}
	if err == nil && st.Completed && st.Provider != "" {
		// Re-run only if preferred provider lost its key and nothing else works
		if _, ok := KeySource(st.Provider); ok || st.Provider == "ollama" {
			return false
		}
		if CountPresentKeys() == 0 {
			return true
		}
		// preferred missing but others exist → let multi-key path re-pick
		return CountPresentKeys() > 0
	}
	n := CountPresentKeys()
	if n == 0 {
		return true
	}
	// Multi-key without completed preference → clarify default
	if n > 1 && (err != nil || !st.Completed || st.Provider == "") {
		return true
	}
	// Single key, first time → short confirm (completed false)
	if n >= 1 && (err != nil || !st.Completed) {
		return true
	}
	return false
}
