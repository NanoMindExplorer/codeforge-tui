package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CODEFORGE_CONFIG_DIR", dir)
	t.Setenv("CODEFORGE_NO_KEYRING", "1")
	t.Setenv("CODEFORGE_SECRETS_BACKEND", "file")
	// clear provider env so tests don't pick up host secrets
	for _, e := range []string{"XAI_API_KEY", "GROK_API_KEY", "GEMINI_API_KEY", "ANTHROPIC_API_KEY", "OPENAI_API_KEY"} {
		t.Setenv(e, "")
	}
	return dir
}

func TestMergeWritePreservesUnknownKeys(t *testing.T) {
	dir := withTempConfig(t)
	path := filepath.Join(dir, "config.yaml")
	initial := `
default_provider: grok
theme: groknight
custom_extension:
  foo: bar
  nested:
    x: 1
experimental_flag: true
providers:
  grok:
    enabled: true
    type: xai
    default_model: grok-4.5
`
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SaveDefaultProvider("gemini"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "default_provider: gemini") {
		t.Fatalf("default not updated:\n%s", s)
	}
	if !strings.Contains(s, "custom_extension:") || !strings.Contains(s, "foo: bar") {
		t.Fatalf("unknown keys lost:\n%s", s)
	}
	if !strings.Contains(s, "experimental_flag: true") {
		t.Fatalf("top-level unknown lost:\n%s", s)
	}
	if !strings.Contains(s, "default_model: grok-4.5") {
		t.Fatalf("provider model lost:\n%s", s)
	}
}

func TestConfigFileMode0600(t *testing.T) {
	dir := withTempConfig(t)
	if err := SaveExample(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "config.yaml")
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	perm := st.Mode().Perm()
	if perm&0o077 != 0 {
		t.Fatalf("config mode %o should be 0600 (no group/other)", perm)
	}
	if perm&0o600 != 0o600 {
		t.Fatalf("config mode %o missing user rw", perm)
	}
}

func TestSaveDefaultProviderDoesNotWriteEnvKeys(t *testing.T) {
	dir := withTempConfig(t)
	t.Setenv("XAI_API_KEY", "env-secret-should-not-land-on-disk")
	// seed yaml without key
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte("default_provider: gemini\ntheme: groknight\n"), 0o600)

	if err := SaveDefaultProvider("grok"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "env-secret") {
		t.Fatalf("env key leaked into yaml:\n%s", data)
	}
	if !strings.Contains(string(data), "default_provider: grok") {
		t.Fatalf("missing default:\n%s", data)
	}
}

func TestSaveProviderKeyFileBackend(t *testing.T) {
	dir := withTempConfig(t)
	if err := SaveProviderKey("gemini", "AIza-test-key", "gemini-2.5-flash"); err != nil {
		t.Fatal(err)
	}
	// key file 0600
	keyPath := filepath.Join(dir, "keys", "gemini.key")
	st, err := os.Stat(keyPath)
	if err != nil {
		t.Fatal("expected keys/gemini.key:", err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("key file mode %o", st.Mode().Perm())
	}
	raw, _ := os.ReadFile(keyPath)
	if !strings.Contains(string(raw), "AIza-test-key") {
		t.Fatal(string(raw))
	}
	// yaml should not contain plaintext key when file backend used
	cfgPath := filepath.Join(dir, "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if strings.Contains(string(data), "AIza-test-key") {
			t.Fatalf("key should not be in yaml:\n%s", data)
		}
	}
	// Load resolves via file store
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["gemini"].APIKey != "AIza-test-key" {
		t.Fatalf("resolved key: %q", cfg.Providers["gemini"].APIKey)
	}
	if cfg.DefaultProvider != "gemini" {
		t.Fatal(cfg.DefaultProvider)
	}
}

func TestPreferEnvOverFileAndConfig(t *testing.T) {
	dir := withTempConfig(t)
	_ = os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`
providers:
  grok:
    api_key: from-yaml
`), 0o600)
	_ = os.MkdirAll(filepath.Join(dir, "keys"), 0o700)
	_ = os.WriteFile(filepath.Join(dir, "keys", "grok.key"), []byte("from-file\n"), 0o600)

	t.Setenv("XAI_API_KEY", "from-env")
	key, src := LocateAPIKey("grok", "from-yaml")
	if key != "from-env" || src != SourceEnv {
		t.Fatalf("got %q %v", key, src)
	}
	if ResolveAPIKey("grok", "from-yaml") != "from-env" {
		t.Fatal("ResolveAPIKey")
	}
}

func TestEnvOnlyBackendRefusesDisk(t *testing.T) {
	_ = withTempConfig(t)
	t.Setenv("CODEFORGE_SECRETS_BACKEND", "env_only")
	err := SaveProviderKey("openai", "sk-test", "")
	if err == nil {
		t.Fatal("expected error for env_only without env")
	}
	if !strings.Contains(err.Error(), "env_only") {
		t.Fatal(err)
	}
}

func TestValidateSandboxAndMode(t *testing.T) {
	cfg := Default()
	if err := Validate(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Sandbox.Profile = "nope"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid sandbox")
	}
	cfg = Default()
	cfg.Permissions.Mode = "yolo"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid mode")
	}
	cfg = Default()
	cfg.Session.AutoCompactPct = 1.5
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid auto_compact")
	}
	cfg = Default()
	cfg.DiffMode = "fancy"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected invalid diff")
	}
}

func TestLoadRejectsInvalidSandbox(t *testing.T) {
	dir := withTempConfig(t)
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte("sandbox:\n  profile: totally-wrong\n"), 0o600)
	_, err := Load()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "sandbox.profile") {
		t.Fatal(err)
	}
}

func TestRoundTripDefaultProviderAndTheme(t *testing.T) {
	dir := withTempConfig(t)
	path := filepath.Join(dir, "config.yaml")
	_ = os.WriteFile(path, []byte(`
default_provider: grok
theme: tokyonight
user_note: keep-me
providers:
  grok:
    enabled: true
    endpoint: https://example.invalid
`), 0o644)

	// chmod fix via SaveDefault
	if err := SaveDefaultProvider("claude"); err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("after write mode %o", st.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	s := string(data)
	if !strings.Contains(s, "user_note: keep-me") {
		t.Fatal("lost user_note")
	}
	if !strings.Contains(s, "endpoint: https://example.invalid") {
		t.Fatal("lost endpoint")
	}
	if !strings.Contains(s, "default_provider: claude") {
		t.Fatal(s)
	}
}
