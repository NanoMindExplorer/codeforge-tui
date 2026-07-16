package github

import "testing"

func TestParseRemoteSlug(t *testing.T) {
	cases := map[string]string{
		"git@github.com:NanoMindExplorer/codeforge.git":     "NanoMindExplorer/codeforge",
		"https://github.com/NanoMindExplorer/codeforge.git": "NanoMindExplorer/codeforge",
		"https://github.com/NanoMindExplorer/codeforge":     "NanoMindExplorer/codeforge",
		"ssh://git@github.com/NanoMindExplorer/codeforge":   "NanoMindExplorer/codeforge",
	}
	for in, want := range cases {
		got, err := parseRemoteSlug(in)
		if err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if got != want {
			t.Fatalf("%s: got %q want %q", in, got, want)
		}
	}
}

func TestFormatEmptyLists(t *testing.T) {
	if FormatPRList(nil) == "" {
		t.Fatal("expected message")
	}
	if FormatIssueList(nil) == "" {
		t.Fatal("expected message")
	}
}

func TestNewClientTokenFromEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "test-token-xyz")
	c := New("/tmp")
	if c.Token != "test-token-xyz" {
		t.Fatalf("token=%q", c.Token)
	}
}
