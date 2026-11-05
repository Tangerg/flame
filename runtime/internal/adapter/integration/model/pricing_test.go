package model

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

func TestPricingUsesProviderAndServedModel(t *testing.T) {
	const model = "claude-opus-5"
	usage := &chat.Usage{InputTokens: 1000, OutputTokens: 250, CacheReadInputTokens: new(int64), CacheWriteInputTokens: new(int64)}
	info, ok := catalog.LookupModel("anthropic", model)
	if !ok {
		t.Fatal("test fixture model missing from catalog")
	}

	got, err := Pricing("anthropic", catalog.Models("anthropic"))("anthropic", model, usage)
	if err != nil {
		t.Fatal(err)
	}
	want, err := info.Pricing.Cost(chat.Usage{InputTokens: 1000, OutputTokens: 250})
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); !ok || usd != want {
		t.Fatalf("Pricing = %v, %t; want %v, true", usd, ok, want)
	}
}

func TestPricingReportsUnknownProviderAsUnavailable(t *testing.T) {
	got, err := Pricing("anthropic", catalog.Models("anthropic"))("does-not-exist", "claude-opus-5", &chat.Usage{InputTokens: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); ok || usd != 0 {
		t.Fatalf("Pricing for unknown provider = %v, %t; want 0, false", usd, ok)
	}
}

func TestPricingPreservesCatalogPricedZero(t *testing.T) {
	got, err := Pricing("anthropic", catalog.Models("anthropic"))("anthropic", "claude-opus-5", &chat.Usage{})
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); !ok || usd != 0 {
		t.Fatalf("Pricing for zero usage = %v, %t; want 0, true", usd, ok)
	}
}

func TestPricingReportsMissingUsageAsUnavailable(t *testing.T) {
	got, err := Pricing("anthropic", catalog.Models("anthropic"))("anthropic", "claude-opus-5", nil)
	if err != nil {
		t.Fatal(err)
	}
	if usd, ok := got.USD(); ok || usd != 0 {
		t.Fatalf("Pricing without reported usage = %v, %t; want 0, false", usd, ok)
	}
}

func TestPricingPreservesUnknownCatalogPricing(t *testing.T) {
	const provider, model = "huggingface", "zai-org/GLM-4.7-Flash"
	entry, found := catalog.LookupModel(provider, model)
	if !found || len(entry.Pricing) != 0 {
		t.Fatal("test fixture must have unknown catalog pricing")
	}
	for _, usage := range []*chat.Usage{{}, {InputTokens: 1000, OutputTokens: 250}} {
		cost, err := Pricing(provider, catalog.Models(provider))(provider, model, usage)
		if err != nil {
			t.Fatal(err)
		}
		if usd, known := cost.USD(); known || usd != 0 {
			t.Fatalf("unknown catalog pricing became free: %v, %t", usd, known)
		}
	}
}

func TestPricingRejectsInvalidCachePartition(t *testing.T) {
	cost, err := Pricing("anthropic", catalog.Models("anthropic"))("anthropic", "claude-opus-5", &chat.Usage{InputTokens: 10, CacheReadInputTokens: new(int64(6)), CacheWriteInputTokens: new(int64(6))})
	if _, known := cost.USD(); err == nil || known {
		t.Fatalf("invalid cache partition became a price: %+v, %v", cost, err)
	}
}

func TestPricingRequiresEvidenceForDifferentlyPricedCacheTokens(t *testing.T) {
	info := catalog.Model{ID: "model", Pricing: catalog.PricingSchedule{{InputPer1M: 10, CacheReadPer1M: new(float64(1))}}}
	price := Pricing("provider", []catalog.Model{info})
	unknown, err := price("provider", "model", &chat.Usage{InputTokens: 100})
	if _, known := unknown.USD(); err != nil || known {
		t.Fatalf("unreported cache: %v, %v", unknown, err)
	}
	known, err := price("provider", "model", &chat.Usage{InputTokens: 100, CacheReadInputTokens: new(int64)})
	if usd, available := known.USD(); err != nil || !available || usd != .001 {
		t.Fatalf("reported zero: %v, %v", known, err)
	}
}
