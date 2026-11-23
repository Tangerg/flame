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

// The upstream catalog spells unknown as zero; this is the one place that
// convention is translated.
func publishedTokenLimit(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}
