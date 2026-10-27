package model

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

// Pricing returns a per-round cost calculator backed by the model
// catalog. It prices the round's provider and served model, so every Run
// reports CostUSD against the model that actually answered.
func Pricing() accounting.Pricing {
	return func(provider, servedModel string, usage *chat.Usage) (accounting.Cost, error) {
		if usage == nil {
			return accounting.Cost{}, nil
		}
		if info, ok := catalog.Default.Lookup(provider, servedModel); ok {
			catalogUsage := catalog.Usage{
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			}
			if usage.CacheReadInputTokens != nil {
				catalogUsage.CacheReadInputTokens = *usage.CacheReadInputTokens
			}
			if usage.CacheWriteInputTokens != nil {
				catalogUsage.CacheWriteInputTokens = *usage.CacheWriteInputTokens
			}
			usd, err := info.Pricing.Cost(catalogUsage)
			if errors.Is(err, catalog.ErrUnknownPricing) {
				return accounting.Cost{}, nil
			}
			if err != nil {
				return accounting.Cost{}, fmt.Errorf("model: price %q/%q: %w", provider, servedModel, err)
			}
			cost, err := accounting.NewCost(usd)
			if err != nil {
				return accounting.Cost{}, fmt.Errorf("model: admit catalog cost: %w", err)
			}
			return cost, nil
		}
		return accounting.Cost{}, nil
	}
}
