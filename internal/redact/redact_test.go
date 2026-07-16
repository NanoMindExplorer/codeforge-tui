package redact

import "testing"

func TestRedactAPIKey(t *testing.T) {
	in := `export GEMINI_API_KEY=AIzaSyDummyKeyValue1234567890abcd`
	out := Redact(in)
	if out == in {
		t.Fatal("expected redaction")
	}
	if !contains(out, "REDACTED") {
		t.Fatalf("%q", out)
	}
}

func TestSensitiveFile(t *testing.T) {
	out, blocked := RedactFile(".env", "SECRET=1")
	if !blocked {
		t.Fatal("expected blocked")
	}
	if !contains(out, "REDACTED") {
		t.Fatal(out)
	}
}

func TestRedactGitHubPAT(t *testing.T) {
	in := "token ghp_abcdefghijklmnopqrstuvwxyz012345"
	out := Redact(in)
	if contains(out, "ghp_abcd") {
		t.Fatalf("pat leaked: %q", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(len(s) > 0 && (stringIndex(s, sub) >= 0)))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
