package provider

import "testing"

func TestCostForModelGeminiPro(t *testing.T) {
	p := NewGeminiProvider("fake", "gemini-2.5-pro")
	cost := CostForModel(p, "gemini-2.5-pro", 1_000_000, 1_000_000)
	if cost < 10 {
		t.Fatalf("expected paid pro cost, got %v", cost)
	}
}

func TestCostForModelClaude(t *testing.T) {
	p := NewClaudeProvider("fake", "claude-sonnet-4-20250514")
	cost := CostForModel(p, "claude-sonnet-4-20250514", 1_000_000, 1_000_000)
	// 3 + 15 = 18
	if cost < 17 || cost > 19 {
		t.Fatalf("cost=%v", cost)
	}
}

func TestSetModel(t *testing.T) {
	p := NewGeminiProvider("x", "gemini-2.5-flash")
	if err := p.SetModel("gemini-2.5-pro"); err != nil {
		t.Fatal(err)
	}
	if p.Model() != "gemini-2.5-pro" {
		t.Fatal(p.Model())
	}
}
