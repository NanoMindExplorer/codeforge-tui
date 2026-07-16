package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Diagnostics runs project checks (go build/vet/test or custom command)
// and returns structured-ish failure output for the agent.
type Diagnostics struct {
	WorkDir string
}

func (d *Diagnostics) Name() string { return "diagnostics" }
func (d *Diagnostics) Description() string {
	return `Run project diagnostics and return compiler/linter/test errors.
Modes: auto (detect), go_build, go_vet, go_test, custom (with command field).
Use after edits to verify the project still builds.`
}

func (d *Diagnostics) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"mode": map[string]any{
				"type":        "string",
				"description": "auto | go_build | go_vet | go_test | custom",
			},
			"command": map[string]any{
				"type":        "string",
				"description": "Shell command when mode=custom",
			},
			"packages": map[string]any{
				"type":        "string",
				"description": "Go packages pattern (default ./...)",
			},
		},
	}
}

type diagInput struct {
	Mode     string `json:"mode"`
	Command  string `json:"command"`
	Packages string `json:"packages"`
}

func (d *Diagnostics) Execute(input json.RawMessage) Result {
	var in diagInput
	_ = json.Unmarshal(input, &in)
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if mode == "" || mode == "auto" {
		mode = detectMode(d.WorkDir)
	}
	pkgs := in.Packages
	if pkgs == "" {
		pkgs = "./..."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var (
		cmd  *exec.Cmd
		label string
	)
	switch mode {
	case "go_build":
		label = "go build " + pkgs
		cmd = exec.CommandContext(ctx, "go", "build", pkgs)
	case "go_vet":
		label = "go vet " + pkgs
		cmd = exec.CommandContext(ctx, "go", "vet", pkgs)
	case "go_test":
		label = "go test " + pkgs
		cmd = exec.CommandContext(ctx, "go", "test", pkgs, "-count=1", "-timeout=60s")
	case "custom":
		if strings.TrimSpace(in.Command) == "" {
			return Result{Error: "command required for custom diagnostics"}
		}
		label = in.Command
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", in.Command)
	default:
		return Result{Error: "unknown mode " + mode}
	}
	cmd.Dir = d.WorkDir
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if len(out) > 12_000 {
		out = out[:12_000] + "\n… (truncated)"
	}
	if err != nil {
		return Result{
			Success: true, // tool ran; project may have errors — still success for agent to read
			Output:  fmt.Sprintf("DIAGNOSTICS FAIL (%s)\n%s\n\n%s", label, err.Error(), out),
		}
	}
	if strings.TrimSpace(out) == "" {
		out = "(clean — no output)"
	}
	return Result{Success: true, Output: fmt.Sprintf("DIAGNOSTICS OK (%s)\n%s", label, out)}
}

func detectMode(workdir string) string {
	if _, err := os.Stat(filepath.Join(workdir, "go.mod")); err == nil {
		return "go_build"
	}
	if _, err := os.Stat(filepath.Join(workdir, "package.json")); err == nil {
		return "custom" // caller may still pass command; default npm test later
	}
	return "go_build"
}

// Enable custom auto for package.json via Execute path when mode auto and no go.mod
func (d *Diagnostics) ExecuteStream(input []byte, progress ProgressFunc) Result {
	if progress != nil {
		progress("running diagnostics…")
	}
	// if auto + package.json without go.mod, use npm test --if-present style
	var in diagInput
	_ = json.Unmarshal(input, &in)
	mode := strings.ToLower(strings.TrimSpace(in.Mode))
	if (mode == "" || mode == "auto") && !fileExists(filepath.Join(d.WorkDir, "go.mod")) &&
		fileExists(filepath.Join(d.WorkDir, "package.json")) {
		in.Mode = "custom"
		if in.Command == "" {
			in.Command = "npm test --silent 2>&1 | head -n 200"
		}
		raw, _ := json.Marshal(in)
		return d.Execute(raw)
	}
	return d.Execute(input)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
