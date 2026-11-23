package contract

import (
	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
	"github.com/Tangerg/flame/plugins/memory/internal/coordination"
)

type CallerPolicy struct {
	SearchToolName         string `json:"searchToolName"`
	MaxModelInputBytes     int    `json:"maxModelInputBytes"`
	MaxModelOutputTokens   int64  `json:"maxModelOutputTokens"`
	MaxModelCalls          int    `json:"maxModelCalls"`
	MaxEmbeddingTexts      int    `json:"maxEmbeddingTexts"`
	MaxEmbeddingInputBytes int64  `json:"maxEmbeddingInputBytes"`
	MaxCallbackInputBytes  int64  `json:"maxCallbackInputBytes"`
	MaxCallbackOutputBytes int64  `json:"maxCallbackOutputBytes"`
}

// The embedding input is a JSON array of every visible text from both targets:
// a code point escapes to at most six bytes, each string adds its quotes and a
// separator, and the brackets add one byte beyond the last omitted separator.
const (
	memoryTargets                = 2
	jsonEscapedBytesPerCodePoint = 6
	jsonStringFramingBytes       = 3
	jsonArrayFramingBytes        = 1
)

func ModelCallerPolicy() CallerPolicy {
	texts := agentmemory.MaxVisiblePerTarget * memoryTargets
	input := int64(texts)*(int64(agentmemory.MaxContentCharacters)*jsonEscapedBytesPerCodePoint+jsonStringFramingBytes) + jsonArrayFramingBytes
	return CallerPolicy{
		SearchToolName:     "search_memory",
		MaxModelInputBytes: coordination.MaximumModelInputBytes, MaxModelOutputTokens: coordination.MaximumModelOutputTokens,
		MaxModelCalls: coordination.MaximumModelCalls, MaxEmbeddingTexts: texts, MaxEmbeddingInputBytes: input,
		MaxCallbackInputBytes: input + 64*1024, MaxCallbackOutputBytes: 64 * 1024 * 1024,
	}
}
