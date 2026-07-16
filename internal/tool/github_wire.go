package tool

import gh "github.com/codeforge/tui/internal/github"

// githubClient is the subset used at registration time.
type githubClient = *gh.Client

func defaultGitHubClient(workDir string) *gh.Client {
	return gh.New(workDir)
}
