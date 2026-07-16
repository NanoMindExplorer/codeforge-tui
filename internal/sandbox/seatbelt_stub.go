//go:build !darwin

package sandbox

// ApplySeatbelt is a no-op outside macOS.
func ApplySeatbelt(e *Engine) (bool, error) {
	return false, nil
}
