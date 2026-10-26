package accounting

import (
	"fmt"

	"github.com/Tangerg/scope/core/chat"
)

// NewTokens folds one provider-reported response into Runtime's cumulative
// counters. [chat.Usage] keeps an unsupported breakdown absent so it cannot be
// read as a reported zero; a cumulative counter has no absent state, because
// aggregating heterogeneous providers would otherwise have to erase a
// dimension one of them never reported. The distinction survives per call in
// the stored [chat.Usage] itself.
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

// CloneReportedUsage detaches the breakdown pointers so a stored value cannot
// be advanced through a caller's copy.
func CloneReportedUsage(usage chat.Usage) chat.Usage {
	if usage.ReasoningTokens != nil {
		usage.ReasoningTokens = new(*usage.ReasoningTokens)
	}
	if usage.CacheReadInputTokens != nil {
		usage.CacheReadInputTokens = new(*usage.CacheReadInputTokens)
	}
	if usage.CacheWriteInputTokens != nil {
		usage.CacheWriteInputTokens = new(*usage.CacheWriteInputTokens)
	}
	return usage
}

func optionalInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
