// Package clipboard copies text to the system clipboard (best-effort).
package clipboard

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// Write tries platform clipboards; falls back to a temp file.
func Write(text string) error {
	if text == "" {
		return fmt.Errorf("empty")
	}
	if p := os.Getenv("CODEFORGE_CLIPBOARD_FILE"); p != "" {
		return os.WriteFile(p, []byte(text), 0644)
	}
	switch runtime.GOOS {
	case "darwin":
		if err := pipeArgs([]string{"pbcopy"}, text); err == nil {
			return nil
		}
	case "windows":
		if err := pipeArgs([]string{"clip"}, text); err == nil {
			return nil
		}
	default:
		if err := pipeArgs([]string{"wl-copy"}, text); err == nil {
			return nil
		}
		if err := pipeArgs([]string{"xclip", "-selection", "clipboard"}, text); err == nil {
			return nil
		}
		if err := pipeArgs([]string{"xsel", "--clipboard", "--input"}, text); err == nil {
			return nil
		}
	}
	path := "/tmp/codeforge-clipboard.txt"
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		return err
	}
	return fmt.Errorf("no clipboard tool; wrote %s", path)
}

func pipeArgs(args []string, text string) error {
	if len(args) == 0 {
		return fmt.Errorf("no cmd")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
