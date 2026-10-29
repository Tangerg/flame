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

	got, err := Pricing("anthropic", catalog.Default.Models("anthropic"))("anthropic", model, usage)
	if err != nil {
		t.Fatal(err)
	}
	want, err := info.Pricing.Cost(catalog.Usage{InputTokens: 1000, OutputTokens: 250})
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); !ok || usd != want {
		t.Fatalf("Pricing = %v, %t; want %v, true", usd, ok, want)
	}
}

func TestPricingReportsUnknownProviderAsUnavailable(t *testing.T) {
	got, err := Pricing("anthropic", catalog.Default.Models("anthropic"))("does-not-exist", "claude-opus-5", &chat.Usage{InputTokens: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); ok || usd != 0 {
		t.Fatalf("Pricing for unknown provider = %v, %t; want 0, false", usd, ok)
	}
}

func TestPricingPreservesCatalogPricedZero(t *testing.T) {
	got, err := Pricing("anthropic", catalog.Default.Models("anthropic"))("anthropic", "claude-opus-5", &chat.Usage{})
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); !ok || usd != 0 {
		t.Fatalf("Pricing for zero usage = %v, %t; want 0, true", usd, ok)
	}
}

func TestPricingReportsMissingUsageAsUnavailable(t *testing.T) {
	got, err := Pricing("anthropic", catalog.Default.Models("anthropic"))("anthropic", "claude-opus-5", nil)
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); ok || usd != 0 {
		t.Fatalf("Pricing without reported usage = %v, %t; want 0, false", usd, ok)
	}
}

func TestPricingPreservesUnknownCatalogPricing(t *testing.T) {
	const provider, model = "huggingface", "zai-org/GLM-4.7-Flash"
	entry, found := catalog.Default.Lookup(provider, model)
	if !found || len(entry.Pricing) != 0 {
		t.Fatal("test fixture must have unknown catalog pricing")
	}
	for _, usage := range []*chat.Usage{{}, {InputTokens: 1000, OutputTokens: 250}} {
		cost, err := Pricing(provider, catalog.Default.Models(provider))(provider, model, usage)
		if err != nil {
			t.Fatal(err)
		}
		if usd, known := cost.USD(); known || usd != 0 {
			t.Fatalf("unknown catalog pricing became free: %v, %t", usd, known)
		}
	}
}

func TestPricingRejectsInvalidCachePartition(t *testing.T) {
	cost, err := Pricing("anthropic", catalog.Default.Models("anthropic"))("anthropic", "claude-opus-5", &chat.Usage{InputTokens: 10, CacheReadInputTokens: new(int64(6)), CacheWriteInputTokens: new(int64(6))})
	if _, known := cost.USD(); err == nil || known {
		t.Fatalf("invalid cache partition became a price: %+v, %v", cost, err)
	}
}
