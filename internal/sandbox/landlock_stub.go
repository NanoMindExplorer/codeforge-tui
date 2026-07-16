//go:build !linux

package sandbox

// ApplyLandlock is a no-op on non-Linux platforms.
func ApplyLandlock(e *Engine) (bool, error) {
	return false, nil
}
