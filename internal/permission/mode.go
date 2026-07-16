package permission

import "strings"

// Mode is the prompt policy for tool authorization.
type Mode string

const (
	// ModeDefault prompts for non-auto-approved tools.
	ModeDefault Mode = "default"
	// ModePlan is design-only (writes denied except plan.md — enforced by SessionDesign too).
	ModePlan Mode = "plan"
	// ModeAlwaysApprove auto-approves (YOLO); deny/hooks/ask-on-shell still apply.
	ModeAlwaysApprove Mode = "always_approve"
	// ModeDontAsk denies anything not explicitly allowed or auto-approved (CI).
	ModeDontAsk Mode = "dont_ask"
)

// ParseMode normalizes config/CLI values.
func ParseMode(s string) Mode {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "plan", "design":
		return ModePlan
	case "always_approve", "always-approve", "always", "yolo", "bypass", "bypasspermissions":
		return ModeAlwaysApprove
	case "dont_ask", "dont-ask", "dontask", "deny_default":
		return ModeDontAsk
	default:
		return ModeDefault
	}
}

func (m Mode) String() string {
	if m == "" {
		return string(ModeDefault)
	}
	return string(m)
}
