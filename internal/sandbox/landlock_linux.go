//go:build linux

package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Linux landlock syscall numbers (asm-generic / arm64 / amd64 modern kernels).
const (
	sysLandlockCreateRuleset = 444
	sysLandlockAddRule       = 445
	sysLandlockRestrictSelf  = 446

	landlockRuleTypePathBeneath = 1
	landlockCreateRulesetVersion = 1 << 0
)

// FS access rights we care about (from linux/landlock.h) as uint64.
const (
	llExecute    uint64 = unix.LANDLOCK_ACCESS_FS_EXECUTE
	llWriteFile  uint64 = unix.LANDLOCK_ACCESS_FS_WRITE_FILE
	llReadFile   uint64 = unix.LANDLOCK_ACCESS_FS_READ_FILE
	llReadDir    uint64 = unix.LANDLOCK_ACCESS_FS_READ_DIR
	llRemoveDir  uint64 = unix.LANDLOCK_ACCESS_FS_REMOVE_DIR
	llRemoveFile uint64 = unix.LANDLOCK_ACCESS_FS_REMOVE_FILE
	llMakeChar   uint64 = unix.LANDLOCK_ACCESS_FS_MAKE_CHAR
	llMakeDir    uint64 = unix.LANDLOCK_ACCESS_FS_MAKE_DIR
	llMakeReg    uint64 = unix.LANDLOCK_ACCESS_FS_MAKE_REG
	llMakeSock   uint64 = unix.LANDLOCK_ACCESS_FS_MAKE_SOCK
	llMakeFifo   uint64 = unix.LANDLOCK_ACCESS_FS_MAKE_FIFO
	llMakeBlock  uint64 = unix.LANDLOCK_ACCESS_FS_MAKE_BLOCK
	llMakeSym    uint64 = unix.LANDLOCK_ACCESS_FS_MAKE_SYM
	llRefer      uint64 = unix.LANDLOCK_ACCESS_FS_REFER
	llTruncate   uint64 = unix.LANDLOCK_ACCESS_FS_TRUNCATE
)

const llWriteAll = llWriteFile | llRemoveDir | llRemoveFile | llMakeChar | llMakeDir |
	llMakeReg | llMakeSock | llMakeFifo | llMakeBlock | llMakeSym | llTruncate

const llReadAll = llExecute | llReadFile | llReadDir | llRefer

// ApplyLandlock installs process-wide Landlock rules for this profile.
// Returns (applied, error). On unsupported kernels returns (false, nil) unless failClosed.
func ApplyLandlock(e *Engine) (bool, error) {
	if e == nil || e.Profile == Off {
		return false, nil
	}
	// Probe ABI
	abi, _, errno := syscall.Syscall(sysLandlockCreateRuleset, 0, 0, landlockCreateRulesetVersion)
	if errno != 0 {
		if e.FailClosed {
			return false, fmt.Errorf("landlock unavailable: %v", errno)
		}
		return false, nil
	}
	_ = abi

	// Which access rights we handle (deny-by-default for these after restrict)
	var handled uint64
	switch e.Profile {
	case Workspace, Devbox, ReadOnly:
		// Restrict writes only; reads stay unrestricted
		handled = llWriteAll
	case Strict:
		handled = llWriteAll | llReadAll
	default:
		return false, nil
	}

	attr := unix.LandlockRulesetAttr{Access_fs: handled}
	ruleset, _, errno := syscall.Syscall(sysLandlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)), unsafe.Sizeof(attr), 0)
	if errno != 0 {
		if e.FailClosed {
			return false, fmt.Errorf("landlock_create_ruleset: %v", errno)
		}
		return false, nil
	}
	rsFD := int(ruleset)
	defer unix.Close(rsFD)

	// Allowed write roots
	writeRoots := []string{}
	switch e.Profile {
	case Workspace, Strict, Devbox:
		writeRoots = append(writeRoots, e.WorkDir, e.CodeforgeHome, "/tmp", "/var/tmp")
		if td := os.TempDir(); td != "" {
			writeRoots = append(writeRoots, td)
		}
	case ReadOnly:
		writeRoots = append(writeRoots, e.CodeforgeHome, "/tmp", "/var/tmp")
		if td := os.TempDir(); td != "" {
			writeRoots = append(writeRoots, td)
		}
	}
	// Home may be needed for ~/.codeforge when nested
	if e.CodeforgeHome != "" {
		writeRoots = append(writeRoots, e.CodeforgeHome)
	}

	allowedWrite := llWriteAll
	for _, root := range uniquePaths(writeRoots) {
		if err := landlockAllowPath(rsFD, root, allowedWrite); err != nil {
			// missing path is OK
			if !os.IsNotExist(err) {
				LogEvent("landlock_path_skip", map[string]any{"path": root, "err": err.Error()})
			}
		}
	}

	if e.Profile == Strict {
		// Allow read on workdir, codeforge, system paths, tmp
		readRoots := []string{
			e.WorkDir, e.CodeforgeHome, "/tmp", "/var/tmp",
			"/usr", "/bin", "/sbin", "/lib", "/lib64", "/etc", "/opt",
			"/proc", "/dev", // needed for basic process ops
		}
		if td := os.TempDir(); td != "" {
			readRoots = append(readRoots, td)
		}
		// Also allow read of Go module cache / home tools if under home
		if home, err := os.UserHomeDir(); err == nil {
			readRoots = append(readRoots, home)
		}
		for _, root := range uniquePaths(readRoots) {
			_ = landlockAllowPath(rsFD, root, llReadAll|allowedWrite)
		}
	}

	// PR_SET_NO_NEW_PRIVS required before restrict_self
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		if e.FailClosed {
			return false, fmt.Errorf("PR_SET_NO_NEW_PRIVS: %w", err)
		}
		return false, nil
	}

	_, _, errno = syscall.Syscall(sysLandlockRestrictSelf, uintptr(rsFD), 0, 0)
	if errno != 0 {
		if e.FailClosed {
			return false, fmt.Errorf("landlock_restrict_self: %v", errno)
		}
		return false, nil
	}
	LogEvent("landlock_applied", map[string]any{
		"profile": string(e.Profile),
		"abi":     int(abi),
	})
	return true, nil
}

func landlockAllowPath(rulesetFD int, path string, access uint64) error {
	path = filepath.Clean(path)
	if path == "" {
		return os.ErrNotExist
	}
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	attr := unix.LandlockPathBeneathAttr{
		Allowed_access: access,
		Parent_fd:      int32(fd),
	}
	_, _, errno := syscall.Syscall6(sysLandlockAddRule,
		uintptr(rulesetFD),
		landlockRuleTypePathBeneath,
		uintptr(unsafe.Pointer(&attr)),
		0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

func uniquePaths(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range in {
		p = filepath.Clean(p)
		if p == "" || p == "." || seen[p] {
			continue
		}
		// resolve abs
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
