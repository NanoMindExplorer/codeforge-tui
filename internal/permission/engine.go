// Package permission implements Grok-style allow/deny/ask authorization.
package permission

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Decision is the result of Evaluate (before interactive ask resolution).
type Decision int

const (
	// DecisionAllow — proceed without prompt.
	DecisionAllow Decision = iota
	// DecisionDeny — hard block.
	DecisionDeny
	// DecisionAsk — prompt the user (or deny in headless/dont_ask).
	DecisionAsk
)

func (d Decision) String() string {
	switch d {
	case DecisionAllow:
		return "allow"
	case DecisionDeny:
		return "deny"
	case DecisionAsk:
		return "ask"
	default:
		return "?"
	}
}

// Result is a full authorization outcome.
type Result struct {
	Decision Decision
	Reason   string
	// RememberKey is the key to store if user chooses "always".
	RememberKey string
	// Dangerous means "always" is not offered / not applied.
	Dangerous bool
}

// AskFunc resolves DecisionAsk. Returns allow, always (remember), error.
// Always is ignored when Dangerous.
type AskFunc func(ctx context.Context, tool, input, reason string, dangerous bool) (allow, always bool, err error)

// HookPre is PreToolUse: return deny=true to block.
type HookPre func(ctx context.Context, tool, input string) (deny bool, reason string)

// HookPost is PostToolUse (observational).
type HookPost func(ctx context.Context, tool, input, output string, success bool)

// Engine evaluates tool calls.
type Engine struct {
	mu    sync.RWMutex
	Mode  Mode
	Rules []Rule
	// Remembered grants: key -> allow (true) or never (false)
	Remembered map[string]bool
	Workdir    string
	// Interactive ask (nil => deny on ask in non-interactive)
	Ask AskFunc
	// Hooks
	PreHooks  []HookPre
	PostHooks []HookPost
	// Headless: treat Ask as Deny
	Headless bool
}

// NewEngine creates an engine with defaults.
func NewEngine(workdir string) *Engine {
	return &Engine{
		Mode:       ModeDefault,
		Rules:      DefaultRules(),
		Remembered: map[string]bool{},
		Workdir:    workdir,
	}
}

// DefaultRules seed safe deny patterns.
func DefaultRules() []Rule {
	return []Rule{
		{Tool: "run_command", Pattern: "rm -rf *", Effect: EffectDeny},
		{Tool: "run_command", Pattern: "rm -fr *", Effect: EffectDeny},
		{Tool: "run_command", Pattern: "sudo *", Effect: EffectAsk},
		{Tool: "run_command", Pattern: "git push --force*", Effect: EffectAsk},
		{Tool: "run_command", Pattern: "git push -f*", Effect: EffectAsk},
	}
}

// SetMode updates the permission mode.
func (e *Engine) SetMode(m Mode) {
	e.mu.Lock()
	e.Mode = m
	e.mu.Unlock()
}

// GetMode returns current mode.
func (e *Engine) GetMode() Mode {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Mode
}

// AddRule appends a rule.
func (e *Engine) AddRule(r Rule) {
	e.mu.Lock()
	e.Rules = append(e.Rules, r)
	e.mu.Unlock()
}

// Evaluate returns allow/deny/ask without prompting.
// Order: hooks → deny rules → ask rules → allow rules → remembered → defaults → mode policy.
func (e *Engine) Evaluate(toolName, input string) Result {
	e.mu.RLock()
	mode := e.Mode
	rules := append([]Rule(nil), e.Rules...)
	remembered := copyMap(e.Remembered)
	e.mu.RUnlock()

	subject := matchSubject(toolName, input)
	dangerous := toolName == "run_command" && IsDangerous(subject)
	key := rememberKey(toolName, subject)

	// 1) Pre-hooks evaluated in Authorize (need ctx) — Evaluate is pure rules.
	// Callers run hooks first.

	// 2) Rules: deny wins
	var askHit, allowHit *Rule
	for i := range rules {
		r := &rules[i]
		if !r.Match(toolName, input) {
			continue
		}
		switch r.Effect {
		case EffectDeny:
			return Result{Decision: DecisionDeny, Reason: fmt.Sprintf("denied by rule %s(%s)", r.Tool, r.Pattern), Dangerous: dangerous, RememberKey: key}
		case EffectAsk:
			if askHit == nil {
				askHit = r
			}
		case EffectAllow:
			if allowHit == nil {
				allowHit = r
			}
		}
	}
	if askHit != nil {
		// shell ask still applies in always_approve
		return Result{Decision: DecisionAsk, Reason: fmt.Sprintf("ask rule %s(%s)", askHit.Tool, askHit.Pattern), Dangerous: dangerous, RememberKey: key}
	}
	if allowHit != nil {
		return Result{Decision: DecisionAllow, Reason: "allowed by rule", Dangerous: dangerous, RememberKey: key}
	}

	// 3) Remembered (skip for dangerous; skip in always_approve for never? Grok: always_approve skips remembered)
	if mode != ModeAlwaysApprove {
		if allow, ok := remembered[key]; ok {
			if allow && !dangerous {
				return Result{Decision: DecisionAllow, Reason: "remembered allow", RememberKey: key}
			}
			if !allow {
				return Result{Decision: DecisionDeny, Reason: "remembered deny", RememberKey: key}
			}
		}
		// also try tool-wide remember
		if allow, ok := remembered[toolName+":*"]; ok && allow && !dangerous {
			return Result{Decision: DecisionAllow, Reason: "remembered tool allow", RememberKey: toolName + ":*"}
		}
	}

	// 4) Built-in auto-approvals
	if IsReadOnlyTool(toolName) {
		return Result{Decision: DecisionAllow, Reason: "read-only tool", RememberKey: key}
	}
	if toolName == "run_command" && IsReadOnlyShell(subject) {
		return Result{Decision: DecisionAllow, Reason: "read-only shell", RememberKey: key}
	}

	// Design mode: non-plan writes should be denied at this layer too
	if mode == ModePlan {
		if isEditTool(toolName) && !isPlanTool(toolName) {
			return Result{Decision: DecisionDeny, Reason: "permission mode plan: project edits denied", RememberKey: key}
		}
	}

	// 5) Mode policy
	switch mode {
	case ModeAlwaysApprove:
		// auto-approve remaining (deny/ask rules already applied)
		return Result{Decision: DecisionAllow, Reason: "always_approve", RememberKey: key, Dangerous: dangerous}
	case ModeDontAsk:
		return Result{Decision: DecisionDeny, Reason: "dont_ask: no explicit allow", RememberKey: key, Dangerous: dangerous}
	default:
		// default: edit tools allowed (BUILD staging / YOLO write path is the safety net);
		// shell (non-readonly), MCP, and github ask once.
		if isEditTool(toolName) {
			return Result{Decision: DecisionAllow, Reason: "default allow edits (staged in BUILD)", RememberKey: key}
		}
		if toolName == "run_command" || strings.HasPrefix(toolName, "mcp_") || toolName == "github" {
			return Result{Decision: DecisionAsk, Reason: "default policy requires approval", RememberKey: key, Dangerous: dangerous}
		}
		return Result{Decision: DecisionAllow, Reason: "default allow", RememberKey: key}
	}
}

// Authorize runs hooks + evaluate + interactive ask. Nil error means allow.
func (e *Engine) Authorize(ctx context.Context, toolName, input string) error {
	// Pre hooks
	e.mu.RLock()
	pres := append([]HookPre(nil), e.PreHooks...)
	e.mu.RUnlock()
	for _, h := range pres {
		if h == nil {
			continue
		}
		if deny, reason := h(ctx, toolName, input); deny {
			if reason == "" {
				reason = "blocked by PreToolUse hook"
			}
			return fmt.Errorf("%s", reason)
		}
	}

	res := e.Evaluate(toolName, input)
	switch res.Decision {
	case DecisionAllow:
		return nil
	case DecisionDeny:
		return fmt.Errorf("permission denied: %s", res.Reason)
	case DecisionAsk:
		e.mu.RLock()
		ask := e.Ask
		headless := e.Headless
		mode := e.Mode
		e.mu.RUnlock()
		if headless || mode == ModeDontAsk || ask == nil {
			return fmt.Errorf("permission denied (would ask): %s", res.Reason)
		}
		allow, always, err := ask(ctx, toolName, input, res.Reason, res.Dangerous)
		if err != nil {
			return err
		}
		if !allow {
			if always && !res.Dangerous {
				e.Remember(res.RememberKey, false)
			}
			return fmt.Errorf("permission denied by user: %s", res.Reason)
		}
		if always && !res.Dangerous {
			e.Remember(res.RememberKey, true)
		}
		return nil
	}
	return nil
}

// NotifyPost runs PostToolUse hooks.
func (e *Engine) NotifyPost(ctx context.Context, toolName, input, output string, success bool) {
	e.mu.RLock()
	posts := append([]HookPost(nil), e.PostHooks...)
	e.mu.RUnlock()
	for _, h := range posts {
		if h != nil {
			h(ctx, toolName, input, output, success)
		}
	}
}

// Remember stores a grant/deny for the project session (+ disk).
func (e *Engine) Remember(key string, allow bool) {
	if key == "" {
		return
	}
	e.mu.Lock()
	if e.Remembered == nil {
		e.Remembered = map[string]bool{}
	}
	e.Remembered[key] = allow
	wd := e.Workdir
	snap := copyMap(e.Remembered)
	e.mu.Unlock()
	_ = saveRemembered(wd, snap)
}

// LoadRemembered loads persisted grants for workdir.
func (e *Engine) LoadRemembered() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if m, err := loadRemembered(e.Workdir); err == nil {
		e.Remembered = m
	}
}

// ClearRemembered wipes session grants.
func (e *Engine) ClearRemembered() {
	e.mu.Lock()
	e.Remembered = map[string]bool{}
	wd := e.Workdir
	e.mu.Unlock()
	_ = saveRemembered(wd, map[string]bool{})
}

// ListRules returns a copy of rules.
func (e *Engine) ListRules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]Rule, len(e.Rules))
	copy(out, e.Rules)
	return out
}

// Summary is human-readable status.
func (e *Engine) Summary() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var b strings.Builder
	fmt.Fprintf(&b, "Permission mode: %s\n", e.Mode)
	fmt.Fprintf(&b, "Rules: %d\n", len(e.Rules))
	for _, r := range e.Rules {
		fmt.Fprintf(&b, "  [%s] %s %q\n", r.Effect, r.Tool, r.Pattern)
	}
	fmt.Fprintf(&b, "Remembered: %d\n", len(e.Remembered))
	for k, v := range e.Remembered {
		a := "deny"
		if v {
			a = "allow"
		}
		fmt.Fprintf(&b, "  %s → %s\n", k, a)
	}
	return b.String()
}

func rememberKey(tool, subject string) string {
	subject = strings.TrimSpace(subject)
	if len(subject) > 120 {
		subject = subject[:120]
	}
	return tool + ":" + subject
}

func isEditTool(name string) bool {
	switch name {
	case "write_file", "search_replace", "apply_patch":
		return true
	}
	return false
}

func isPlanTool(name string) bool {
	switch name {
	case "write_plan", "exit_plan_mode", "enter_plan_mode":
		return true
	}
	return false
}

func copyMap(m map[string]bool) map[string]bool {
	out := make(map[string]bool, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func rememberPath(workdir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	// hash workdir into filename
	safe := strings.ReplaceAll(workdir, string(filepath.Separator), "_")
	if len(safe) > 80 {
		safe = safe[len(safe)-80:]
	}
	dir := filepath.Join(home, ".codeforge", "permissions")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "remember_"+safe+".json"), nil
}

func saveRemembered(workdir string, m map[string]bool) error {
	path, err := rememberPath(workdir)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func loadRemembered(workdir string) (map[string]bool, error) {
	path, err := rememberPath(workdir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]bool{}, err
	}
	var m map[string]bool
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]bool{}, err
	}
	if m == nil {
		m = map[string]bool{}
	}
	return m, nil
}
