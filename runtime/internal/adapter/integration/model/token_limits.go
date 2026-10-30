package model

import (
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/models/catalog"
)

func catalogTokenLimits(entry catalog.Model) (modelref.TokenLimits, error) {
	return modelref.NewTokenLimits(modelref.TokenLimitValues{
		ContextWindow:   publishedTokenLimit(entry.Limits.ContextWindow),
		MaxInputTokens:  publishedTokenLimit(entry.Limits.MaxInputTokens),
		MaxOutputTokens: publishedTokenLimit(entry.Limits.MaxOutputTokens),
	})
}

// publishedTokenLimit translates the upstream catalog's legacy numeric
// absence convention exactly once at the infrastructure boundary. Domain and
// application code never receive zero as an alternate spelling of unknown.
func publishedTokenLimit(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
