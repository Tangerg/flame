package model

import (
	"github.com/Tangerg/scope/models/catalog"
	"testing"
)

func TestCatalogTokenLimitsPreserveIndependentFacts(t *testing.T) {
	limits, err := catalogTokenLimits(catalog.Model{Limits: catalog.Limits{ContextWindow: 400000, MaxInputTokens: 272000, MaxOutputTokens: 272000}})
	if err != nil {
		t.Fatal(err)
	}
	c, ck := limits.ContextWindow()
	i, ik := limits.MaxInputTokens()
	o, ok := limits.MaxOutputTokens()
	if c != 400000 || !ck || i != 272000 || !ik || o != 272000 || !ok {
		t.Fatalf("captured limits changed: %+v", limits)
	}
	unknown, err := catalogTokenLimits(catalog.Model{})
	if err != nil || !unknown.Unknown() {
		t.Fatalf("unknown=%+v err=%v", unknown, err)
	}
}
