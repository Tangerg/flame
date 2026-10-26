package contract

import (
	app "github.com/Tangerg/flame/plugins/memory/internal/application"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

// CallerPolicy publishes the resource envelope for the Memory workflow. Core
// consumes generated values rather than importing private business capacity.
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

func ModelCallerPolicy() CallerPolicy {
	// Six bytes per code point cover worst-case JSON escaping in both targets.
	texts := memory.MaxVisiblePerTarget * 2
	input := int64(texts)*(int64(memory.MaxContentCharacters)*6+3) + 1
	return CallerPolicy{
		SearchToolName:     "search_memory",
		MaxModelInputBytes: app.MaximumModelInputBytes, MaxModelOutputTokens: app.MaximumModelOutputTokens,
		MaxModelCalls: app.MaximumModelCalls, MaxEmbeddingTexts: texts, MaxEmbeddingInputBytes: input,
		MaxCallbackInputBytes: input + 64*1024, MaxCallbackOutputBytes: 64 * 1024 * 1024,
	}
}
