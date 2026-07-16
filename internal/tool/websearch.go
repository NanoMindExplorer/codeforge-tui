package tool

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/codeforge/tui/internal/redact"
)

// WebSearch performs a lightweight web search (DuckDuckGo Instant Answer + related).
// Optional BRAVE_API_KEY for richer results when set.
type WebSearch struct{}

func (w *WebSearch) Name() string { return "web_search" }
func (w *WebSearch) Description() string {
	return `Search the public web for current information, docs, or error messages.
Returns titles, snippets, and URLs. Prefer this before guessing API behavior.
Grok-compatible tool name.`
}
func (w *WebSearch) Schema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Search query",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Max results (default 5, max 10)",
			},
		},
		"required": []string{"query"},
	}
}

type webSearchInput struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

func (w *WebSearch) Execute(input json.RawMessage) Result {
	var in webSearchInput
	if err := json.Unmarshal(input, &in); err != nil {
		return Result{Error: err.Error()}
	}
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return Result{Error: "query required"}
	}
	max := in.MaxResults
	if max <= 0 {
		max = 5
	}
	if max > 10 {
		max = 10
	}

	if k := strings.TrimSpace(os.Getenv("BRAVE_API_KEY")); k != "" {
		if out, err := braveSearch(k, q, max); err == nil && out != "" {
			return Result{Success: true, Output: redact.Redact(out)}
		}
	}

	out, err := duckDuckGoSearch(q, max)
	if err != nil {
		return Result{Error: err.Error()}
	}
	return Result{Success: true, Output: redact.Redact(out)}
}

func duckDuckGoSearch(q string, max int) (string, error) {
	u := "https://api.duckduckgo.com/?q=" + url.QueryEscape(q) + "&format=json&no_html=1&skip_disambig=1"
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return "", fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("search HTTP %d", resp.StatusCode)
	}
	var data struct {
		AbstractText   string `json:"AbstractText"`
		AbstractURL    string `json:"AbstractURL"`
		Heading        string `json:"Heading"`
		Answer         string `json:"Answer"`
		RelatedTopics  []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
			Topics   []struct {
				Text     string `json:"Text"`
				FirstURL string `json:"FirstURL"`
			} `json:"Topics"`
		} `json:"RelatedTopics"`
		Results []struct {
			Text     string `json:"Text"`
			FirstURL string `json:"FirstURL"`
		} `json:"Results"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Web search: %q\n\n", q)
	if data.Answer != "" {
		fmt.Fprintf(&b, "Answer: %s\n\n", data.Answer)
	}
	if data.AbstractText != "" {
		fmt.Fprintf(&b, "## %s\n%s\n%s\n\n", data.Heading, data.AbstractText, data.AbstractURL)
	}
	n := 0
	add := func(text, link string) {
		if n >= max || strings.TrimSpace(text) == "" {
			return
		}
		n++
		fmt.Fprintf(&b, "%d. %s\n   %s\n", n, text, link)
	}
	for _, r := range data.Results {
		add(r.Text, r.FirstURL)
	}
	for _, rt := range data.RelatedTopics {
		if rt.Text != "" {
			add(rt.Text, rt.FirstURL)
		}
		for _, t := range rt.Topics {
			add(t.Text, t.FirstURL)
		}
	}
	if n == 0 && data.AbstractText == "" && data.Answer == "" {
		b.WriteString("(No structured results — try a more specific query or web_fetch a known docs URL.)\n")
	}
	return b.String(), nil
}

func braveSearch(apiKey, q string, max int) (string, error) {
	u := "https://api.search.brave.com/res/v1/web/search?q=" + url.QueryEscape(q) + "&count=" + fmt.Sprintf("%d", max)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", apiKey)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("brave HTTP %d: %s", resp.StatusCode, string(raw))
	}
	var data struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Web search (Brave): %q\n\n", q)
	for i, r := range data.Web.Results {
		if i >= max {
			break
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.Description, r.URL)
	}
	if len(data.Web.Results) == 0 {
		return "", fmt.Errorf("no results")
	}
	return b.String(), nil
}
