package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// SecretsBackend selects where API keys are persisted (Q3.2 / Q3.3).
//
// Priority when *resolving* a key (never write path):
//  1. Process environment (XAI_API_KEY, …) — always wins
//  2. OS keyring (if backend is keyring/auto and available)
//  3. File keystore ~/.config/codeforge/keys/<provider> (mode 0600)
//  4. providers.<name>.api_key in config.yaml (discouraged plaintext)
//
// Prefer env in CI and production. Config-file keys are optional convenience
// for local machines; disk compromise can leak them.
type SecretsBackend string

const (
	SecretsAuto    SecretsBackend = "auto"     // keyring if available, else file
	SecretsFile    SecretsBackend = "file"     // 0600 files under keys/
	SecretsKeyring SecretsBackend = "keyring"  // OS keyring; fall back to file on error
	SecretsEnvOnly SecretsBackend = "env_only" // never persist secrets to disk
)

// SecretsConfig is the [secrets] section of config.yaml.
type SecretsConfig struct {
	// Backend: auto | file | keyring | env_only (default auto).
	Backend SecretsBackend `mapstructure:"backend" yaml:"backend"`
}

// EnvKeysFor returns environment variable names checked for a provider.
func EnvKeysFor(provider string) []string {
	switch normalizeProviderName(provider) {
	case "grok":
		return []string{"XAI_API_KEY", "GROK_API_KEY"}
	case "gemini":
		return []string{"GEMINI_API_KEY"}
	case "claude":
		return []string{"ANTHROPIC_API_KEY"}
	case "openai":
		return []string{"OPENAI_API_KEY"}
	default:
		return nil
	}
}

// EnvAPIKey returns the first non-empty env key for provider, or "".
func EnvAPIKey(provider string) string {
	for _, e := range EnvKeysFor(provider) {
		if v := strings.TrimSpace(os.Getenv(e)); v != "" {
			return v
		}
	}
	return ""
}

// ResolveAPIKey implements the prefer-env policy (Q3.2).
// cfgKey is the value from config.yaml (may be empty).
func ResolveAPIKey(provider, cfgKey string) string {
	if v := EnvAPIKey(provider); v != "" {
		return v
	}
	provider = normalizeProviderName(provider)
	if v, err := loadSecretStore(provider); err == nil && v != "" {
		return v
	}
	return strings.TrimSpace(cfgKey)
}

// StoreAPIKey persists a key according to secrets.backend (Q3.3).
// Never stores when an env var already supplies the same provider (no-op success)
// if preferEnv is true — avoids duplicating secrets onto disk.
func StoreAPIKey(provider, apiKey string, backend SecretsBackend, preferEnv bool) error {
	provider = normalizeProviderName(provider)
	apiKey = strings.TrimSpace(apiKey)
	if provider == "" || apiKey == "" {
		return fmt.Errorf("provider and api key required")
	}
	if preferEnv && EnvAPIKey(provider) != "" {
		// Env already provides the key — do not write to disk.
		return nil
	}
	b := effectiveBackend(backend)
	switch b {
	case SecretsEnvOnly:
		return fmt.Errorf("secrets.backend=env_only: refusing to write API key to disk (export %s instead)",
			firstEnvName(provider))
	case SecretsKeyring:
		if err := keyringSet(provider, apiKey); err == nil {
			return nil
		}
		// fall through to file
		return fileKeySet(provider, apiKey)
	case SecretsFile, SecretsAuto:
		// auto: try keyring first
		if b == SecretsAuto {
			if err := keyringSet(provider, apiKey); err == nil {
				return nil
			}
		}
		return fileKeySet(provider, apiKey)
	default:
		return fileKeySet(provider, apiKey)
	}
}

func effectiveBackend(b SecretsBackend) SecretsBackend {
	// Env override for ops / CI
	if v := strings.TrimSpace(strings.ToLower(os.Getenv("CODEFORGE_SECRETS_BACKEND"))); v != "" {
		return SecretsBackend(v)
	}
	if b == "" {
		return SecretsAuto
	}
	return SecretsBackend(strings.ToLower(string(b)))
}

func firstEnvName(provider string) string {
	ks := EnvKeysFor(provider)
	if len(ks) == 0 {
		return "API_KEY"
	}
	return ks[0]
}

func keysDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "keys"), nil
}

func fileKeyPath(provider string) (string, error) {
	dir, err := keysDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, normalizeProviderName(provider)+".key"), nil
}

func fileKeySet(provider, apiKey string) error {
	path, err := fileKeyPath(provider)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), DirModeConfig); err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(apiKey+"\n"), FileModeConfig)
}

func fileKeyGet(provider string) (string, error) {
	path, err := fileKeyPath(provider)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// --- OS keyring (optional; soft-fail) ---------------------------------------

var (
	keyringMu      sync.Mutex
	keyringService = "codeforge"
)

// keyringSet stores via OS keyring when available. Returns error if unavailable.
// Implementation is in secrets_keyring.go (real) / can no-op when disabled.
func keyringSet(provider, apiKey string) error {
	return keyringSetImpl(provider, apiKey)
}

func keyringGet(provider string) (string, error) {
	return keyringGetImpl(provider)
}

func loadSecretStore(provider string) (string, error) {
	// keyring first, then file
	if v, err := keyringGet(provider); err == nil && v != "" {
		return v, nil
	}
	return fileKeyGet(provider)
}

// KeyStorageWarning is shown when keys land in config.yaml plaintext.
const KeyStorageWarning = `API keys in config.yaml are stored in plaintext (mode 0600).
Prefer environment variables (XAI_API_KEY, GEMINI_API_KEY, …) or:
  secrets:
    backend: file      # ~/.config/codeforge/keys/<provider>.key (0600)
    # backend: keyring # OS keyring when available
    # backend: env_only # never write secrets to disk
See docs/ONBOARDING.md · docs/SECRETS.md`

// SecretSource describes where a resolved key came from (for /provider UX).
type SecretSource string

const (
	SourceEnv     SecretSource = "env"
	SourceKeyring SecretSource = "keyring"
	SourceFile    SecretSource = "file"
	SourceConfig  SecretSource = "config"
	SourceNone    SecretSource = "none"
)

// LocateAPIKey reports where the active key for provider would come from.
func LocateAPIKey(provider, cfgKey string) (key string, src SecretSource) {
	if v := EnvAPIKey(provider); v != "" {
		return v, SourceEnv
	}
	provider = normalizeProviderName(provider)
	if v, err := keyringGet(provider); err == nil && v != "" {
		return v, SourceKeyring
	}
	if v, err := fileKeyGet(provider); err == nil && v != "" {
		return v, SourceFile
	}
	if strings.TrimSpace(cfgKey) != "" {
		return strings.TrimSpace(cfgKey), SourceConfig
	}
	return "", SourceNone
}
