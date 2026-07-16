//go:build darwin

package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ApplySeatbelt writes a Seatbelt profile and re-execs under sandbox-exec once.
// Controlled by env to avoid loops: CODEFORGE_SEATBELT_APPLIED=1.
func ApplySeatbelt(e *Engine) (bool, error) {
	if e == nil || e.Profile == Off {
		return false, nil
	}
	if os.Getenv("CODEFORGE_SEATBELT_APPLIED") == "1" {
		return true, nil // already inside sandbox-exec
	}
	if os.Getenv("CODEFORGE_SEATBELT") == "0" {
		return false, nil
	}

	profile, err := buildSeatbeltProfile(e)
	if err != nil {
		return false, err
	}
	dir := filepath.Join(e.CodeforgeHome, "sandbox")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return false, err
	}
	path := filepath.Join(dir, "seatbelt-"+string(e.Profile)+".sb")
	if err := os.WriteFile(path, []byte(profile), 0644); err != nil {
		return false, err
	}
	LogEvent("seatbelt_profile", map[string]any{"path": path, "profile": string(e.Profile)})

	// Re-exec self under sandbox-exec
	bin, err := os.Executable()
	if err != nil {
		return false, err
	}
	args := append([]string{"-f", path, bin}, os.Args[1:]...)
	cmd := exec.Command("sandbox-exec", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "CODEFORGE_SEATBELT_APPLIED=1")
	// Replace process
	if err := cmd.Run(); err != nil {
		if e.FailClosed {
			return false, fmt.Errorf("sandbox-exec: %w", err)
		}
		return false, nil
	}
	// sandbox-exec child exited — exit parent with same code
	os.Exit(cmd.ProcessState.ExitCode())
	return true, nil
}

func buildSeatbeltProfile(e *Engine) (string, error) {
	wd := e.WorkDir
	cf := e.CodeforgeHome
	var b strings.Builder
	b.WriteString("(version 1)\n")
	b.WriteString("(deny default)\n")
	b.WriteString("(allow process*)\n")
	b.WriteString("(allow signal)\n")
	b.WriteString("(allow sysctl-read)\n")
	b.WriteString("(allow mach*)\n")
	b.WriteString("(allow ipc*)\n")
	b.WriteString("(allow network*)\n") // child net block is linux-only in Grok
	// Always allow reading system + writing tmp/codeforge
	b.WriteString("(allow file-read*\n")
	b.WriteString("  (subpath \"/usr\") (subpath \"/bin\") (subpath \"/sbin\")\n")
	b.WriteString("  (subpath \"/System\") (subpath \"/Library\") (subpath \"/Applications\")\n")
	b.WriteString("  (subpath \"/private/var/db\") (subpath \"/dev\") (subpath \"/etc\")\n")
	fmt.Fprintf(&b, "  (subpath %q)\n", cf)
	fmt.Fprintf(&b, "  (subpath \"/tmp\") (subpath \"/private/tmp\") (subpath \"/var/tmp\")\n")

	switch e.Profile {
	case Strict:
		fmt.Fprintf(&b, "  (subpath %q)\n", wd)
		b.WriteString(")\n")
		b.WriteString("(allow file-write*\n")
		fmt.Fprintf(&b, "  (subpath %q) (subpath %q)\n", wd, cf)
		b.WriteString("  (subpath \"/tmp\") (subpath \"/private/tmp\") (subpath \"/var/tmp\")\n)\n")
	case ReadOnly:
		// read everywhere for read-only is hard in seatbelt deny-default;
		// allow home + workdir read, write only codeforge+tmp
		home, _ := os.UserHomeDir()
		if home != "" {
			fmt.Fprintf(&b, "  (subpath %q)\n", home)
		}
		fmt.Fprintf(&b, "  (subpath %q)\n", wd)
		b.WriteString(")\n")
		b.WriteString("(allow file-write*\n")
		fmt.Fprintf(&b, "  (subpath %q)\n", cf)
		b.WriteString("  (subpath \"/tmp\") (subpath \"/private/tmp\") (subpath \"/var/tmp\")\n)\n")
	default: // workspace / devbox
		home, _ := os.UserHomeDir()
		if home != "" {
			fmt.Fprintf(&b, "  (subpath %q)\n", home)
		}
		fmt.Fprintf(&b, "  (subpath %q)\n", wd)
		// workspace: allow broad read of /
		b.WriteString("  (subpath \"/\")\n)\n")
		b.WriteString("(allow file-write*\n")
		fmt.Fprintf(&b, "  (subpath %q) (subpath %q)\n", wd, cf)
		b.WriteString("  (subpath \"/tmp\") (subpath \"/private/tmp\") (subpath \"/var/tmp\")\n)\n")
		if e.Profile == Devbox {
			// allow write most places except /data style - on mac N/A
		}
	}
	return b.String(), nil
}
