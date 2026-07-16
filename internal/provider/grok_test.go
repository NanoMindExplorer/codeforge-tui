package provider

import (
	"strings"
	"testing"
)

func TestGrokProviderDefaults(t *testing.T) {
	t.Setenv("XAI_API_KEY", "xai-test")
	t.Setenv("GROK_MODEL", "")
	p := NewGrokProvider("", "")
	if p.Name() != "grok" {
		t.Fatal(p.Name())
	}
	if p.Model() != "grok-4.5" {
		t.Fatal(p.Model())
	}
	if !strings.Contains(p.endpoint, "api.x.ai") {
		t.Fatal(p.endpoint)
	}
	models := p.Models()
	if len(models) < 1 || models[0].ID != "grok-4.5" {
		t.Fatal(models)
	}
	if err := p.ValidateConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestGrokCostCatalog(t *testing.T) {
	p := NewGrokProvider("xai-x", "grok-4.5")
	cost := CostForModel(p, "grok-4.5", 1_000_000, 1_000_000)
	// 2 + 6 = 8
	if cost < 7.9 || cost > 8.1 {
		t.Fatal(cost)
	}
}
