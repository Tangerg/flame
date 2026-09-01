package model

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

func TestPricingUsesProviderAndServedModel(t *testing.T) {
	const model = "claude-opus-5"
	usage := &chat.Usage{InputTokens: 1000, OutputTokens: 250}
	info, ok := catalog.Default.Lookup("anthropic", model)
	if !ok {
		t.Fatal("test fixture model missing from catalog")
	}

	got := Pricing()("anthropic", model, usage)
	want := info.Pricing.Cost(catalog.Usage{InputTokens: 1000, OutputTokens: 250})
	if usd, ok := got.USD(); !ok || usd != want {
		t.Fatalf("Pricing = %v, %t; want %v, true", usd, ok, want)
	}
}

func TestPricingReportsUnknownProviderAsUnavailable(t *testing.T) {
	got := Pricing()("does-not-exist", "claude-opus-5", &chat.Usage{InputTokens: 1000})
	if usd, ok := got.USD(); ok || usd != 0 {
		t.Fatalf("Pricing for unknown provider = %v, %t; want 0, false", usd, ok)
	}
}

func TestPricingPreservesCatalogPricedZero(t *testing.T) {
	got := Pricing()("anthropic", "claude-opus-5", &chat.Usage{})
	if usd, ok := got.USD(); !ok || usd != 0 {
		t.Fatalf("Pricing for zero usage = %v, %t; want 0, true", usd, ok)
	}
}
