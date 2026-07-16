package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CheckWrite returns an error if path may not be written under the profile (soft + always).
func (e *Engine) CheckWrite(path string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.Profile == Off {
		return nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if e.isDenied(abs) {
		return fmt.Errorf("sandbox deny: write blocked for %s", abs)
	}
	if e.writable(abs) {
		return nil
	}
	return fmt.Errorf("sandbox %s: write outside allowed roots: %s", e.Profile, abs)
}

// CheckRead returns an error if path may not be read (strict only for soft layer).
func (e *Engine) CheckRead(path string) error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.Profile == Off || e.Profile == Workspace || e.Profile == Devbox || e.Profile == ReadOnly {
		// read everywhere (Grok: workspace/read-only/devbox)
		if e.isDenied(path) {
			abs, _ := filepath.Abs(path)
			return fmt.Errorf("sandbox deny: read blocked for %s", abs)
		}
		return nil
	}
	// strict: CWD + system paths + codeforge
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if e.isDenied(abs) {
		return fmt.Errorf("sandbox deny: read blocked for %s", abs)
	}
	if e.readableStrict(abs) {
		return nil
	}
	return fmt.Errorf("sandbox strict: read outside allowed roots: %s", abs)
}

func (e *Engine) writable(abs string) bool {
	switch e.Profile {
	case Workspace, Strict:
		return under(abs, e.WorkDir) || under(abs, e.CodeforgeHome) || e.tempWritable(abs)
	case ReadOnly:
		// project is NOT writable; only codeforge + temp (outside CWD)
		return under(abs, e.CodeforgeHome) || e.tempWritable(abs)
	case Devbox:
		// write almost everywhere except /data and virtual FS
		if under(abs, "/data") || under(abs, "/proc") || under(abs, "/sys") {
			return false
		}
		return true
	default:
		return true
	}
}

// tempWritable allows OS temp dirs but never treats the project workdir as temp
// (tests and Termux often place CWD under $TMPDIR).
func (e *Engine) tempWritable(abs string) bool {
	if under(abs, e.WorkDir) {
		return false
	}
	return isTemp(abs)
}

func (e *Engine) readableStrict(abs string) bool {
	if under(abs, e.WorkDir) || under(abs, e.CodeforgeHome) || isTemp(abs) {
		return true
	}
	// essential system paths for compilers/tools
	for _, p := range []string{"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/opt", "/var/lib"} {
		if under(abs, p) {
			return true
		}
	}
	return false
}

func (e *Engine) isDenied(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	for _, d := range e.Deny {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if matchDeny(d, abs, e.WorkDir) {
			return true
		}
	}
	return false
}

func under(abs, root string) bool {
	if root == "" {
		return false
	}
	abs = filepath.Clean(abs)
	root = filepath.Clean(root)
	if abs == root {
		return true
	}
	prefix := root + string(filepath.Separator)
	return strings.HasPrefix(abs, prefix)
}

func isTemp(abs string) bool {
	for _, t := range []string{"/tmp", "/var/tmp"} {
		if under(abs, t) {
			return true
		}
	}
	// OS temp (Android/Termux often uses $PREFIX/tmp)
	if td := os.TempDir(); td != "" && under(abs, td) {
		return true
	}
	// macOS-style
	if strings.Contains(abs, "/T/") || strings.HasPrefix(abs, "/private/var/folders/") {
		return true
	}
	return false
}

// matchDeny supports exact paths and simple ** globs (Grok subset).
func matchDeny(pattern, abs, workdir string) bool {
	pattern = filepath.ToSlash(pattern)
	abs = filepath.ToSlash(abs)
	if !strings.HasPrefix(pattern, "/") && !strings.ContainsAny(pattern, "*?[") {
		// relative exact under workdir
		cand := filepath.ToSlash(filepath.Join(workdir, pattern))
		return abs == cand || strings.HasPrefix(abs, cand+"/")
	}
	if !strings.ContainsAny(pattern, "*?[") {
		p := filepath.ToSlash(filepath.Clean(pattern))
		return abs == p || strings.HasPrefix(abs, p+"/")
	}
	// glob: **/.env, **/*.pem, *.pem
	return matchStarGlob(pattern, abs, workdir)
}

func matchStarGlob(pattern, abs, workdir string) bool {
	// Anchor relative globs at workdir
	rel := abs
	if workdir != "" {
		if r, err := filepath.Rel(workdir, abs); err == nil && !strings.HasPrefix(r, "..") {
			rel = filepath.ToSlash(r)
		}
	}
	// try both full abs and rel
	for _, target := range []string{abs, rel, filepath.Base(abs)} {
		if globMatch(pattern, target) {
			return true
		}
	}
	// **/.env style
	if strings.HasPrefix(pattern, "**/") {
		suf := pattern[3:]
		if strings.HasPrefix(suf, "*.") {
			return strings.HasSuffix(abs, suf[1:]) || strings.HasSuffix(filepath.Base(abs), suf[1:])
		}
		return filepath.Base(abs) == suf || strings.HasSuffix(abs, "/"+suf)
	}
	if strings.HasPrefix(pattern, "*.") {
		return strings.HasSuffix(abs, pattern[1:])
	}
	return false
}

func globMatch(pattern, name string) bool {
	ok, err := filepath.Match(pattern, name)
	return err == nil && ok
}
