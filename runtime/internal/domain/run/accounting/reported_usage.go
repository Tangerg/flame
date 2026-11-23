package accounting

import (
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

// A cumulative counter has no absent state: aggregating heterogeneous providers
// would otherwise erase a dimension one never reported; the per-call chat.Usage
// keeps the distinction.
func NewTokens(usage chat.Usage) (Tokens, error) {
	if err := usage.Validate(); err != nil {
		return Tokens{}, fmt.Errorf("accounting: reported usage: %w", err)
	}
	tokens := Tokens{
		InputTokens:      usage.InputTokens,
		OutputTokens:     usage.OutputTokens,
		ReasoningTokens:  optionalInt64(usage.ReasoningTokens),
		CacheReadTokens:  optionalInt64(usage.CacheReadInputTokens),
		CacheWriteTokens: optionalInt64(usage.CacheWriteInputTokens),
	}
	if err := tokens.Validate(); err != nil {
		return Tokens{}, err
	}
	return tokens, nil
}

func optionalInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
