// Package secrets redacts credentials before they reach the model or logs.
package redact

import (
	"regexp"
	"strings"
)

var patterns = []*regexp.Regexp{
	// AWS
	regexp.MustCompile(`(?i)(AKIA[0-9A-Z]{16})`),
	regexp.MustCompile(`(?i)(aws_secret_access_key\s*[=:]\s*)\S+`),
	// Generic API keys / tokens
	regexp.MustCompile(`(?i)((?:api[_-]?key|apikey|secret|token|password|passwd|auth|bearer|credential)["'\s:=]+)([^\s"'\\]{8,})`),
	// GitHub / Slack / Google-ish
	regexp.MustCompile(`\b(ghp_[A-Za-z0-9_]{20,})\b`),
	regexp.MustCompile(`\b(gho_[A-Za-z0-9_]{20,})\b`),
	regexp.MustCompile(`\b(github_pat_[A-Za-z0-9_]{20,})\b`),
	regexp.MustCompile(`\b(sk-[A-Za-z0-9_\-]{20,})\b`),
	regexp.MustCompile(`\b(xox[baprs]-[A-Za-z0-9\-]{10,})\b`),
	regexp.MustCompile(`\b(AIza[0-9A-Za-z_\-]{20,})\b`),
	// Private keys
	regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----[\s\S]*?-----END (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
	// JWT-ish
	regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\b`),
}

// SensitiveName reports filenames that should not be fully sent to models.
func SensitiveName(name string) bool {
	n := strings.ToLower(name)
	switch {
	case n == ".env" || strings.HasPrefix(n, ".env."):
		return true
	case strings.HasSuffix(n, ".pem"), strings.HasSuffix(n, ".key"), strings.HasSuffix(n, ".p12"), strings.HasSuffix(n, ".pfx"):
		return true
	case n == "id_rsa", n == "id_ed25519", n == "credentials.json", n == "service-account.json":
		return true
	case strings.Contains(n, "secret") && (strings.HasSuffix(n, ".yml") || strings.HasSuffix(n, ".yaml") || strings.HasSuffix(n, ".json")):
		return true
	}
	return false
}

// Redact replaces known secret patterns with placeholders.
func Redact(s string) string {
	if s == "" {
		return s
	}
	out := s
	for _, re := range patterns {
		out = re.ReplaceAllStringFunc(out, func(m string) string {
			// Keep short prefix key labels when group-captured style
			if strings.Contains(strings.ToLower(m), "key") || strings.Contains(strings.ToLower(m), "token") ||
				strings.Contains(strings.ToLower(m), "secret") || strings.Contains(strings.ToLower(m), "password") ||
				strings.Contains(strings.ToLower(m), "bearer") {
				// leave left side if "key=VALUE"
				if i := strings.IndexAny(m, "=:"); i > 0 && i < len(m)-1 {
					return m[:i+1] + " [REDACTED]"
				}
			}
			return "[REDACTED]"
		})
	}
	return out
}

// RedactFile returns content safe for the model; for sensitive names, returns a stub.
func RedactFile(name, content string) (string, bool) {
	if SensitiveName(name) {
		return "[REDACTED: sensitive file " + name + " — contents not sent to the model]", true
	}
	return Redact(content), false
}
