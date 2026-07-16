package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// HasBubblewrap reports whether bwrap is on PATH.
func HasBubblewrap() bool {
	if runtime.GOOS != "linux" && runtime.GOOS != "android" {
		return false
	}
	_, err := exec.LookPath("bwrap")
	return err == nil
}

// Command builds an *exec.Cmd that runs shellCommand under the sandbox.
// When profile is off or soft backend, runs plain /bin/sh -c.
func (e *Engine) Command(ctx context.Context, shellCommand string) (*exec.Cmd, error) {
	return e.CommandIn(ctx, e.WorkDir, shellCommand)
}

// CommandIn is like Command but chdirs to workdir (does not mutate global WorkDir).
func (e *Engine) CommandIn(ctx context.Context, workdir, shellCommand string) (*exec.Cmd, error) {
	e.mu.RLock()
	profile := e.Profile
	backend := e.Backend
	restrictNet := e.RestrictNetwork
	cf := e.CodeforgeHome
	deny := append([]string(nil), e.Deny...)
	e.mu.RUnlock()

	if workdir == "" {
		workdir = e.WorkDir
	}
	if abs, err := filepath.Abs(workdir); err == nil {
		workdir = abs
	}

	shell, flag := "/bin/sh", "-c"
	if runtime.GOOS == "windows" {
		shell, flag = "cmd", "/C"
	}

	if profile == Off || backend == BackendOff || backend == BackendSoft {
		cmd := exec.CommandContext(ctx, shell, flag, shellCommand)
		cmd.Dir = workdir
		// Soft network: try unshare -n when restrict_network and unshare exists
		if restrictNet && backend == BackendSoft {
			if path, err := exec.LookPath("unshare"); err == nil {
				cmd = exec.CommandContext(ctx, path, "-n", "--", shell, flag, shellCommand)
				cmd.Dir = workdir
			}
		}
		return cmd, nil
	}

	// bubblewrap hard isolation
	tmp := &Engine{
		Profile: profile, WorkDir: workdir, CodeforgeHome: cf,
		Deny: deny, RestrictNetwork: restrictNet, Backend: backend,
	}
	args, err := tmp.bwrapArgs(shell, flag, shellCommand)
	if err != nil {
		return nil, err
	}
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		cmd := exec.CommandContext(ctx, shell, flag, shellCommand)
		cmd.Dir = workdir
		return cmd, nil
	}
	return exec.CommandContext(ctx, bwrap, args...), nil
}

func (e *Engine) bwrapArgs(shell, flag, shellCommand string) ([]string, error) {
	wd := e.WorkDir
	cf := e.CodeforgeHome
	if err := os.MkdirAll(cf, 0755); err != nil {
		return nil, err
	}
	// ensure tmp exists
	_ = os.MkdirAll("/tmp", 0755)

	args := []string{
		"--die-with-parent",
		"--proc", "/proc",
		"--dev", "/dev",
	}

	switch e.Profile {
	case Workspace, Devbox:
		// read everywhere, write workdir + codeforge + tmp
		args = append(args,
			"--ro-bind", "/", "/",
			"--bind", wd, wd,
			"--bind", cf, cf,
			"--bind", "/tmp", "/tmp",
		)
		if st, err := os.Stat("/var/tmp"); err == nil && st.IsDir() {
			args = append(args, "--bind", "/var/tmp", "/var/tmp")
		}
		if e.Profile == Devbox {
			// remount /data as ro empty if exists
			if st, err := os.Stat("/data"); err == nil && st.IsDir() {
				args = append(args, "--ro-bind", "/data", "/data")
			}
		}
	case ReadOnly:
		args = append(args,
			"--ro-bind", "/", "/",
			"--bind", cf, cf,
			"--bind", "/tmp", "/tmp",
		)
		if st, err := os.Stat("/var/tmp"); err == nil && st.IsDir() {
			args = append(args, "--bind", "/var/tmp", "/var/tmp")
		}
		// workdir stays ro via root ro-bind
	case Strict:
		// minimal system + workdir rw
		for _, p := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/opt"} {
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				args = append(args, "--ro-bind", p, p)
			}
		}
		// dynamic linker paths sometimes under /lib
		args = append(args,
			"--bind", wd, wd,
			"--bind", cf, cf,
			"--bind", "/tmp", "/tmp",
		)
		if st, err := os.Stat("/var/tmp"); err == nil && st.IsDir() {
			args = append(args, "--bind", "/var/tmp", "/var/tmp")
		}
		// need /etc/resolv.conf etc already via /etc
	default:
		return nil, fmt.Errorf("unknown profile %s", e.Profile)
	}

	// Deny: bind empty tmpfs or ro empty over path (best-effort for existing paths)
	for _, d := range e.Deny {
		d = strings.TrimSpace(d)
		if d == "" || strings.ContainsAny(d, "*?[") {
			continue // globs expanded at soft layer; exact only for bwrap
		}
		p := d
		if !filepath.IsAbs(p) {
			p = filepath.Join(wd, p)
		}
		if st, err := os.Stat(p); err == nil {
			if st.IsDir() {
				args = append(args, "--tmpfs", p)
			} else {
				// bind-over with /dev/null style: use ro-bind of empty file
				// bwrap --ro-bind /dev/null path works for files
				args = append(args, "--ro-bind", "/dev/null", p)
			}
		}
	}

	if e.RestrictNetwork {
		args = append(args, "--unshare-net")
	}

	args = append(args, "--chdir", wd, "--", shell, flag, shellCommand)
	return args, nil
}

// SoftNote returns a warning string when using soft backend (for tool output header).
func (e *Engine) SoftNote() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.Backend != BackendSoft || e.Profile == Off {
		return ""
	}
	return fmt.Sprintf("[sandbox soft/%s — install bubblewrap for kernel FS isolation]\n", e.Profile)
}
