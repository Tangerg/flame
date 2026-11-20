package model

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
	"github.com/samber/lo"
)

// Pricing returns a per-round cost calculator backed by the model
// catalog. It prices the round's provider and served model, so every Run
// reports CostUSD against the model that actually answered.
func Pricing(providerID string, models []catalog.Model) accounting.Pricing {
	owned := slices.Clone(models)
	for index, model := range owned {
		owned[index] = model.Clone()
	}
	return func(provider, servedModel string, usage *chat.Usage) (accounting.Cost, error) {
		if usage == nil || provider != providerID {
			return accounting.Cost{}, nil
		}
		index := slices.IndexFunc(owned, func(model catalog.Model) bool { return model.ID == servedModel })
		if index < 0 {
			return accounting.Cost{}, nil
		}
		cost, err := catalogCost(owned[index].Pricing, *usage)
		if err != nil {
			return accounting.Cost{}, fmt.Errorf("model: price %q/%q: %w", provider, servedModel, err)
		}
		return cost, nil
	}
}

// catalogCost prices usage by the catalog rate card, or returns the zero Cost
// when the price cannot be known. A missing cache breakdown could cover any
// unallocated input tokens; the catalog owns rates and band selection, so the
// endpoints of its linear formula are compared instead of copying it.
func catalogCost(pricing catalog.PricingSchedule, usage chat.Usage) (accounting.Cost, error) {
	cost, err := admittedCost(pricing, usage)
	if errors.Is(err, catalog.ErrUnknownPricing) {
		return accounting.Cost{}, nil
	}
	if err != nil {
		return accounting.Cost{}, err
	}
	remaining := usage.InputTokens - lo.FromPtr(usage.CacheReadInputTokens) - lo.FromPtr(usage.CacheWriteInputTokens)
	if remaining <= 0 {
		return cost, nil
	}
	var alternatives []chat.Usage
	if usage.CacheReadInputTokens == nil {
		alternative := usage
		alternative.CacheReadInputTokens = new(remaining)
		alternatives = append(alternatives, alternative)
	}
	if usage.CacheWriteInputTokens == nil {
		alternative := usage
		alternative.CacheWriteInputTokens = new(remaining)
		alternatives = append(alternatives, alternative)
	}
	for _, alternative := range alternatives {
		alternativeCost, err := admittedCost(pricing, alternative)
		if err != nil {
			return accounting.Cost{}, err
		}
		if !cost.Equal(alternativeCost) {
			return accounting.Cost{}, nil
		}
	}
	return cost, nil
}

func admittedCost(pricing catalog.PricingSchedule, usage chat.Usage) (accounting.Cost, error) {
	usd, err := pricing.Cost(usage)
	if err != nil {
		return accounting.Cost{}, err
	}
	return accounting.NewCost(usd)
}
