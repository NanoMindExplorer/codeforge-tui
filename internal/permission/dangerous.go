package permission

import (
	"strings"
	"unicode"
)

// Dangerous commands never use "always remember" — always re-prompt on ask.
var dangerousPrefixes = []string{
	"rm ", "rm\t", "rm-",
	"sudo ",
	"mkfs",
	"dd ",
	"shutdown", "reboot", "halt", "poweroff",
	"chmod 777", "chmod -R 777",
	"chown -R",
	"curl | sh", "curl|sh", "wget | sh", "wget|sh",
	"curl | bash", "wget | bash",
	"> /dev/sd",
	"mkfs.",
	":(){", // fork bomb
	"git push --force", "git push -f",
	"git reset --hard",
	"git clean -fd", "git clean -xfd",
	"npm publish",
	"docker system prune",
	"kubectl delete",
	"drop table", "drop database",
	"format c:",
}

// IsDangerous reports whether a shell command (or any subject) is on the dangerous list.
func IsDangerous(subject string) bool {
	s := strings.ToLower(strings.TrimSpace(subject))
	if s == "" {
		return false
	}
	// normalize multiple spaces
	s = strings.Join(strings.Fields(s), " ")
	for _, p := range dangerousPrefixes {
		if strings.Contains(s, p) || strings.HasPrefix(s, strings.TrimSpace(p)) {
			return true
		}
	}
	// bare rm with flags
	if tokens := strings.Fields(s); len(tokens) > 0 {
		base := tokens[0]
		if base == "rm" {
			for _, t := range tokens[1:] {
				if strings.Contains(t, "r") && strings.Contains(t, "f") {
					return true
				}
				if t == "-rf" || t == "-fr" || t == "-r" && containsFlag(tokens, "f") {
					return true
				}
			}
		}
	}
	return false
}

func containsFlag(tokens []string, letter string) bool {
	for _, t := range tokens {
		if strings.HasPrefix(t, "-") && strings.Contains(t, letter) {
			return true
		}
	}
	return false
}

// SplitShellSegments splits on && || ; | for per-segment checks.
func SplitShellSegments(cmd string) []string {
	var segs []string
	var b strings.Builder
	runes := []rune(cmd)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		// && ||
		if r == '&' && i+1 < len(runes) && runes[i+1] == '&' {
			flushSeg(&segs, &b)
			i++
			continue
		}
		if r == '|' && i+1 < len(runes) && runes[i+1] == '|' {
			flushSeg(&segs, &b)
			i++
			continue
		}
		if r == ';' || r == '|' {
			flushSeg(&segs, &b)
			continue
		}
		b.WriteRune(r)
	}
	flushSeg(&segs, &b)
	return segs
}

func flushSeg(segs *[]string, b *strings.Builder) {
	s := strings.TrimSpace(b.String())
	b.Reset()
	if s != "" {
		*segs = append(*segs, s)
	}
}

// PrimaryCommand returns the first word of a shell segment.
func PrimaryCommand(seg string) string {
	seg = strings.TrimSpace(seg)
	for i, r := range seg {
		if unicode.IsSpace(r) {
			return seg[:i]
		}
	}
	return seg
}
