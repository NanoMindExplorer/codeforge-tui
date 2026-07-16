// Package sandbox provides Grok-compatible OS shell sandbox profiles.
//
// Profiles match Grok Build semantics (workspace / read-only / strict / off).
// Enforcement is best-effort: bubblewrap when available (Linux), otherwise a
// soft policy layer on file tools + optional unshare-net for network block.
package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Profile is a named sandbox policy.
type Profile string

const (
	Off      Profile = "off"
	Workspace Profile = "workspace"
	ReadOnly Profile = "read-only"
	Strict   Profile = "strict"
	Devbox   Profile = "devbox"
)

// ParseProfile normalizes user input (flag, env, slash).
func ParseProfile(s string) (Profile, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	switch s {
	case "", "off", "none", "false", "0":
		return Off, true
	case "workspace", "ws", "default":
		return Workspace, true
	case "read-only", "readonly", "ro":
		return ReadOnly, true
	case "strict", "untrusted":
		return Strict, true
	case "devbox", "dev":
		return Devbox, true
	default:
		return "", false
	}
}

// Backend describes how limits are enforced.
type Backend string

const (
	BackendOff   Backend = "off"
	BackendBwrap Backend = "bwrap"
	BackendSoft  Backend = "soft"
)

// Engine is the process-wide sandbox configuration.
type Engine struct {
	mu sync.RWMutex

	Profile  Profile
	WorkDir  string
	// CodeforgeHome is ~/.codeforge (session + memory writes always allowed).
	CodeforgeHome string
	// Deny is extra denied path globs (soft always; bwrap bind-over when possible).
	Deny []string
	// RestrictNetwork blocks child network when backend supports it.
	RestrictNetwork bool
	// Backend selected at Activate time.
	Backend Backend
	// FailClosed refuses soft fallback when true (custom profiles with deny).
	FailClosed bool
}

var (
	globalMu sync.RWMutex
	global   *Engine
)

// Global returns the active engine (never nil after Ensure).
func Global() *Engine {
	globalMu.RLock()
	e := global
	globalMu.RUnlock()
	if e != nil {
		return e
	}
	return Ensure(Off, "")
}

// SetGlobal installs the process sandbox.
func SetGlobal(e *Engine) {
	globalMu.Lock()
	global = e
	globalMu.Unlock()
}

// Ensure creates/replaces the global engine for workdir + profile.
func Ensure(p Profile, workdir string) *Engine {
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(workdir); err == nil {
		workdir = abs
	}
	home, _ := os.UserHomeDir()
	cf := filepath.Join(home, ".codeforge")
	e := &Engine{
		Profile:       p,
		WorkDir:       workdir,
		CodeforgeHome: cf,
	}
	// Built-in network policy matches Grok
	switch p {
	case ReadOnly, Strict:
		e.RestrictNetwork = true
	}
	e.Activate()
	SetGlobal(e)
	return e
}

// Activate picks backend and applies built-in network flags.
func (e *Engine) Activate() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.Profile == Off {
		e.Backend = BackendOff
		return
	}
	if HasBubblewrap() {
		e.Backend = BackendBwrap
		return
	}
	e.Backend = BackendSoft
}

// Summary is a one-line status for UI / logs.
func (e *Engine) Summary() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.Profile == Off {
		return "sandbox: off"
	}
	net := "net=allow"
	if e.RestrictNetwork {
		net = "net=block"
	}
	return fmt.Sprintf("sandbox: %s (%s, %s)", e.Profile, e.Backend, net)
}

// Label short badge for status bar.
func (e *Engine) Label() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	switch e.Profile {
	case Off:
		return ""
	case Workspace:
		return "SBX:ws"
	case ReadOnly:
		return "SBX:ro"
	case Strict:
		return "SBX:strict"
	case Devbox:
		return "SBX:dev"
	default:
		return "SBX"
	}
}

// AllowsNetwork reports whether child processes may use the network.
func (e *Engine) AllowsNetwork() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.Profile == Off {
		return true
	}
	return !e.RestrictNetwork
}

// IsOff is true when no sandbox policy is active.
func (e *Engine) IsOff() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Profile == Off
}

// ResolveFromEnv picks profile: flag > CODEFORGE_SANDBOX > GROK_SANDBOX > config > off.
func ResolveFromEnv(flagVal, configVal string) Profile {
	for _, s := range []string{flagVal, os.Getenv("CODEFORGE_SANDBOX"), os.Getenv("GROK_SANDBOX"), configVal} {
		if p, ok := ParseProfile(s); ok && s != "" {
			return p
		}
		// empty string from flag means "not set"; ParseProfile("") → off which is valid
	}
	// only config empty → off
	if p, ok := ParseProfile(configVal); ok {
		return p
	}
	return Off
}

// ResolvePreferExplicit prefers non-empty flag over env/config.
func ResolvePreferExplicit(flagSet bool, flagVal, configVal string) Profile {
	if flagSet {
		if p, ok := ParseProfile(flagVal); ok {
			return p
		}
		return Off
	}
	for _, s := range []string{os.Getenv("CODEFORGE_SANDBOX"), os.Getenv("GROK_SANDBOX"), configVal} {
		if strings.TrimSpace(s) == "" {
			continue
		}
		if p, ok := ParseProfile(s); ok {
			return p
		}
	}
	return Off
}
