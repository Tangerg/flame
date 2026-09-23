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
	usage.ReasoningTokens = cloneOptionalInt64(usage.ReasoningTokens)
	usage.CacheReadInputTokens = cloneOptionalInt64(usage.CacheReadInputTokens)
	usage.CacheWriteInputTokens = cloneOptionalInt64(usage.CacheWriteInputTokens)
	return usage
}

// ReportedUsageEqual compares two reported values. [chat.Usage] carries its
// breakdowns as pointers, so == would compare addresses and an absent
// dimension would read as equal to a reported zero.
func ReportedUsageEqual(left, right chat.Usage) bool {
	return left.InputTokens == right.InputTokens &&
		left.OutputTokens == right.OutputTokens &&
		equalOptionalInt64(left.ReasoningTokens, right.ReasoningTokens) &&
		equalOptionalInt64(left.CacheReadInputTokens, right.CacheReadInputTokens) &&
		equalOptionalInt64(left.CacheWriteInputTokens, right.CacheWriteInputTokens)
}

func optionalInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func equalOptionalInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func cloneOptionalInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
