package rpc

import memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"

const MaximumModelInput = 512 * 1024
const MaximumModelOutput int64 = 2048

// CallerPolicy publishes the resource envelope for the Memory workflow. Core
// consumes generated values rather than importing private business capacity.
type CallerPolicy struct {
	MaxModelInputBytes     int   `json:"maxModelInputBytes"`
	MaxModelOutputTokens   int64 `json:"maxModelOutputTokens"`
	MaxModelCalls          int   `json:"maxModelCalls"`
	MaxEmbeddingTexts      int   `json:"maxEmbeddingTexts"`
	MaxEmbeddingInputBytes int64 `json:"maxEmbeddingInputBytes"`
	MaxCallbackInputBytes  int64 `json:"maxCallbackInputBytes"`
	MaxCallbackOutputBytes int64 `json:"maxCallbackOutputBytes"`
}

func ModelCallerPolicy() CallerPolicy {
	// Six bytes per code point cover worst-case JSON escaping in both targets.
	texts := memory.MaxVisiblePerTarget * 2
	input := int64(texts)*(int64(memory.MaxContentCharacters)*6+3) + 1
	return CallerPolicy{
		MaxModelInputBytes: MaximumModelInput, MaxModelOutputTokens: MaximumModelOutput,
		MaxModelCalls: 2, MaxEmbeddingTexts: texts, MaxEmbeddingInputBytes: input,
		MaxCallbackInputBytes: input + 64*1024, MaxCallbackOutputBytes: 64 * 1024 * 1024,
	}
}
