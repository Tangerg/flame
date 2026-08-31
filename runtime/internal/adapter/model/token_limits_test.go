package model

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

func TestLookupTokenLimitsPreservesIndependentCatalogFacts(t *testing.T) {
	selection, err := modelref.New("openai", "gpt-5-pro")
	if err != nil {
		t.Fatal(err)
	}
	limits, found, err := LookupTokenLimits(selection)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("LookupTokenLimits() missed openai/gpt-5-pro")
	}
	contextWindow, contextKnown := limits.ContextWindow()
	maxInput, inputKnown := limits.MaxInputTokens()
	maxOutput, outputKnown := limits.MaxOutputTokens()
	if contextWindow != 400_000 || !contextKnown ||
		maxInput != 272_000 || !inputKnown ||
		maxOutput != 272_000 || !outputKnown {
		t.Fatalf(
			"limits = context:(%d,%t) input:(%d,%t) output:(%d,%t)",
			contextWindow, contextKnown,
			maxInput, inputKnown,
			maxOutput, outputKnown,
		)
	}
}

func TestLookupTokenLimitsAllowsPrivateCatalogMiss(t *testing.T) {
	selection, err := modelref.New("openai-compatible", "private-model")
	if err != nil {
		t.Fatal(err)
	}
	limits, found, err := LookupTokenLimits(selection)
	if err != nil || found || !limits.Unknown() {
		t.Fatalf("LookupTokenLimits() = (%+v,%t,%v), want unknown,false,nil", limits, found, err)
	}
}
