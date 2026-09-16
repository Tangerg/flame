package sqlite

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/strictjson"
)

type modelInvocationUsageRow struct {
	InputTokens      *int64 `json:"inputTokens"`
	OutputTokens     *int64 `json:"outputTokens"`
	CacheReadTokens  *int64 `json:"cacheReadTokens"`
	CacheWriteTokens *int64 `json:"cacheWriteTokens"`
	ReasoningTokens  *int64 `json:"reasoningTokens"`
}

func encodeModelInvocationUsage(usage *accounting.TokenUsage) (*string, error) {
	if usage == nil {
		return nil, nil
	}
	if err := usage.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: model invocation usage: %w", err)
	}
	row := modelInvocationUsageRow{
		InputTokens: &usage.PromptTokens, OutputTokens: &usage.CompletionTokens,
		CacheReadTokens: &usage.CacheReadTokens, CacheWriteTokens: &usage.CacheWriteTokens, ReasoningTokens: &usage.ReasoningTokens,
	}
	data, err := json.Marshal(row)
	if err != nil {
		return nil, fmt.Errorf("sqlite: encode model invocation usage: %w", err)
	}
	encoded := string(data)
	return &encoded, nil
}

func decodeModelInvocationUsage(encoded string) (*accounting.TokenUsage, error) {
	if err := strictjson.ValidateUniqueMembers([]byte(encoded)); err != nil {
		return nil, fmt.Errorf("sqlite: decode model invocation usage: %w", err)
	}
	var row *modelInvocationUsageRow
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&row); err != nil {
		return nil, fmt.Errorf("sqlite: decode model invocation usage: %w", err)
	}
	if row == nil || row.InputTokens == nil || row.OutputTokens == nil || row.CacheReadTokens == nil || row.CacheWriteTokens == nil || row.ReasoningTokens == nil {
		return nil, errors.New("sqlite: model invocation usage requires all token counts")
	}
	usage := &accounting.TokenUsage{PromptTokens: *row.InputTokens, CompletionTokens: *row.OutputTokens, CacheReadTokens: *row.CacheReadTokens, CacheWriteTokens: *row.CacheWriteTokens, ReasoningTokens: *row.ReasoningTokens}
	if err := usage.Validate(); err != nil {
		return nil, fmt.Errorf("sqlite: restore model invocation usage: %w", err)
	}
	return usage, nil
}
