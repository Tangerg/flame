package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/strictjson"
)

// Both totals are reported on every call, so their absence is a corrupt
// record. A breakdown is null exactly when the provider does not report that
// dimension, which is not the same fact as a reported zero.
type modelInvocationUsageRow struct {
	InputTokens      *int64 `json:"inputTokens"`
	OutputTokens     *int64 `json:"outputTokens"`
	CacheReadTokens  *int64 `json:"cacheReadTokens"`
	CacheWriteTokens *int64 `json:"cacheWriteTokens"`
	ReasoningTokens  *int64 `json:"reasoningTokens"`
}

func encodeModelInvocationUsage(usage *chat.Usage) (*string, error) {
	if usage == nil {
		return nil, nil
	}
	if err := usage.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: model invocation usage: %w", err)
	}
	row := modelInvocationUsageRow{
		InputTokens: &usage.InputTokens, OutputTokens: &usage.OutputTokens,
		CacheReadTokens: usage.CacheReadInputTokens, CacheWriteTokens: usage.CacheWriteInputTokens,
		ReasoningTokens: usage.ReasoningTokens,
	}
	data, err := json.Marshal(row)
	if err != nil {
		return nil, fmt.Errorf("sqlite: encode model invocation usage: %w", err)
	}
	encoded := string(data)
	return &encoded, nil
}

func decodeModelInvocationUsage(encoded string) (*chat.Usage, error) {
	if err := strictjson.ValidateUniqueMembers([]byte(encoded)); err != nil {
		return nil, fmt.Errorf("sqlite: decode model invocation usage: %w", err)
	}
	var row *modelInvocationUsageRow
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&row); err != nil {
		return nil, fmt.Errorf("sqlite: decode model invocation usage: %w", err)
	}
	if row == nil || row.InputTokens == nil || row.OutputTokens == nil {
		return nil, errors.New("sqlite: model invocation usage requires both token totals")
	}
	usage := &chat.Usage{
		InputTokens: *row.InputTokens, OutputTokens: *row.OutputTokens,
		ReasoningTokens: row.ReasoningTokens, CacheReadInputTokens: row.CacheReadTokens,
		CacheWriteInputTokens: row.CacheWriteTokens,
	}
	if err := usage.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: restore model invocation usage: %w", err)
	}
	return usage, nil
}
