package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

// keyringSetImpl stores provider API key in the OS keyring (Q3.3).
// Disabled when CODEFORGE_SECRETS_BACKEND=file|env_only or CODEFORGE_NO_KEYRING=1.
func keyringSetImpl(provider, apiKey string) error {
	if keyringDisabled() {
		return fmt.Errorf("keyring disabled")
	}
	provider = normalizeProviderName(provider)
	keyringMu.Lock()
	defer keyringMu.Unlock()
	return keyring.Set(keyringService, provider, apiKey)
}

func keyringGetImpl(provider string) (string, error) {
	if keyringDisabled() {
		return "", fmt.Errorf("keyring disabled")
	}
	provider = normalizeProviderName(provider)
	keyringMu.Lock()
	defer keyringMu.Unlock()
	secret, err := keyring.Get(keyringService, provider)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(secret), nil
}

func keyringDisabled() bool {
	if v := strings.TrimSpace(os.Getenv("CODEFORGE_NO_KEYRING")); v == "1" || strings.EqualFold(v, "true") {
		return true
	}
	b := strings.TrimSpace(strings.ToLower(os.Getenv("CODEFORGE_SECRETS_BACKEND")))
	return b == "file" || b == "env_only"
}
