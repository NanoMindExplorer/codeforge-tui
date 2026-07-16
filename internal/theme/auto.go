package theme

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// DetectSystemLight returns true if the OS prefers light appearance.
// Fallbacks: COLORFGBG light bg, then dark default.
func DetectSystemLight() bool {
	// Explicit override
	if v := os.Getenv("CODEFORGE_APPEARANCE"); v != "" {
		switch strings.ToLower(v) {
		case "light", "day":
			return true
		case "dark", "night":
			return false
		}
	}
	switch runtime.GOOS {
	case "darwin":
		return detectMacOSLight()
	case "linux":
		if light, ok := detectLinuxPortal(); ok {
			return light
		}
	case "windows":
		if light, ok := detectWindowsLight(); ok {
			return light
		}
	}
	// COLORFGBG=fg;bg — high bg number ≈ light
	if cfg := os.Getenv("COLORFGBG"); cfg != "" {
		parts := strings.Split(cfg, ";")
		if len(parts) >= 2 {
			bg := strings.TrimSpace(parts[len(parts)-1])
			// common: 15 or 7 = light bg
			if bg == "15" || bg == "7" || bg == "11" {
				return true
			}
			if bg == "0" || bg == "8" {
				return false
			}
		}
	}
	return false // default dark (GrokNight)
}

func detectMacOSLight() bool {
	// AppleInterfaceStyle is "Dark" when dark; absent when light
	cmd := exec.Command("defaults", "read", "-g", "AppleInterfaceStyle")
	out, err := cmd.Output()
	if err != nil {
		// key missing → light
		return true
	}
	return !strings.Contains(strings.ToLower(string(out)), "dark")
}

func detectLinuxPortal() (light bool, ok bool) {
	// org.freedesktop.appearance color-scheme: 0 default, 1 prefer-dark, 2 prefer-light
	cmd := exec.Command("gdbus", "call", "--session",
		"--dest", "org.freedesktop.portal.Desktop",
		"--object-path", "/org/freedesktop/portal/desktop",
		"--method", "org.freedesktop.portal.Settings.Read",
		"org.freedesktop.appearance", "color-scheme")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		// try dbus-send
		return detectLinuxDBusSend()
	}
	s := string(out)
	if strings.Contains(s, "uint32 1") || strings.Contains(s, "1>") {
		return false, true // prefer dark
	}
	if strings.Contains(s, "uint32 2") || strings.Contains(s, "2>") {
		return true, true // prefer light
	}
	return false, false
}

func detectLinuxDBusSend() (bool, bool) {
	cmd := exec.Command("dbus-send", "--session", "--print-reply=literal",
		"--dest=org.freedesktop.portal.Desktop",
		"/org/freedesktop/portal/desktop",
		"org.freedesktop.portal.Settings.Read",
		"string:org.freedesktop.appearance",
		"string:color-scheme")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, false
	}
	s := string(out)
	if strings.Contains(s, " 1") {
		return false, true
	}
	if strings.Contains(s, " 2") {
		return true, true
	}
	return false, false
}

func detectWindowsLight() (bool, bool) {
	// AppsUseLightTheme registry via powershell (best-effort)
	cmd := exec.Command("powershell", "-NoProfile", "-Command",
		"(Get-ItemProperty -Path 'HKCU:\\Software\\Microsoft\\Windows\\CurrentVersion\\Themes\\Personalize').AppsUseLightTheme")
	out, err := cmd.Output()
	if err != nil {
		return false, false
	}
	s := strings.TrimSpace(string(out))
	if s == "1" {
		return true, true
	}
	if s == "0" {
		return false, true
	}
	return false, false
}

// AutoPollMsg is emitted every few seconds when auto theme is enabled.
type AutoPollMsg struct {
	Time time.Time
}

// AutoPollInterval is how often to re-check system appearance.
const AutoPollInterval = 5 * time.Second
