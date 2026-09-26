package protocol

// UtilityRole is the (provider, model) the in-house maintenance services run
// on (models.getUtilityRole / setUtilityRole). Empty model uses the Runtime
// composition's default selection. Provider must be configured when assigned.
// The selection remains stored if credentials later change, so clients that
// need effective availability join it with providers.list.
// Saving affects the next utility invocation, including during an existing Run.
// It does not replace that Run's main-model deployment or an in-flight invocation.
type UtilityRole struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// EmbeddingRole is the optional (provider, model) used to enrich agent-memory
// ranking (models.getEmbeddingRole / setEmbeddingRole). Empty model leaves
// memory search keyword-only. Provider must be configured and embedding-capable when
// the role is assigned. The selection remains stored if credentials later
// change, so clients that need effective availability join it with
// providers.list. A distinct type from [UtilityRole] (same shape, different
// domain — under the rule-of-three for sharing).
// Saving affects subsequent searches, without rewriting earlier search results.
type EmbeddingRole struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

// ListModelsRequest is the models.list body. Provider is optional
// (models are organized by provider; omitted → empty page).
type ListModelsRequest struct {
	Provider string `json:"provider,omitempty"`
}

// Model is one entry in models.list. A provider's complete page contains each
// model ID once in ascending ID order.
type Model struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	DisplayName string `json:"displayName,omitempty"`
	// TokenLimits is omitted when the provider publishes no context-envelope
	// facts. Every present member is strictly positive and at least one member
	// must be present; numeric zero is never a wire spelling of unknown.
	TokenLimits *ModelTokenLimits `json:"tokenLimits,omitzero"`
	// KnowledgeCutoff is the training cutoff (RFC3339 date), empty when unknown.
	KnowledgeCutoff string `json:"knowledgeCutoff,omitempty"`
	// Deprecated marks a model the provider has retired; clients hide or flag it.
	Deprecated   bool               `json:"deprecated,omitzero"`
	Capabilities *ModelCapabilities `json:"capabilities,omitzero"`
	Pricing      *ModelPricing      `json:"pricing,omitzero"`
}

// ModelTokenLimits is the provider-published context envelope for one exact
// model. The three maxima are independent facts rather than simultaneously
// attainable quotas. A streaming/multimodal model may legitimately publish an
// output maximum above its input context window.
type ModelTokenLimits struct {
	ContextWindow   *int64 `json:"contextWindow,omitzero"`
	MaxInputTokens  *int64 `json:"maxInputTokens,omitzero"`
	MaxOutputTokens *int64 `json:"maxOutputTokens,omitzero"`
}

// Modality is a media type a model takes as input or emits as output
// It mirrors core's chat.Modality. The enum is open, so new media types
// are added without bumping the contract.
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
	ModalityPDF   Modality = "pdf"
)

// ModelCapabilities describes per-model capabilities. The booleans are
// quick gates; the list / level fields carry the detail a model picker needs
// (which media the model accepts, what reasoning effort levels it offers).
type ModelCapabilities struct {
	// Reasoning reports whether the model supports extended thinking at all.
	Reasoning bool `json:"reasoning,omitzero"`
	// ReasoningLevels are the discrete effort levels the model accepts, in
	// increasing order (e.g. ["low","medium","high"]). Empty when reasoning is
	// budget-controlled (no discrete levels) or unsupported.
	ReasoningLevels []string `json:"reasoningLevels,omitempty"`
	// ReasoningDefaultLevel is the effort used when the caller picks none;
	// empty when there are no levels.
	ReasoningDefaultLevel string `json:"reasoningDefaultLevel,omitempty"`
	// Multimodal is a convenience flag: the model accepts image input. The
	// full set is InputModalities.
	Multimodal bool `json:"multimodal,omitzero"`
	// InputModalities lists every media type the model accepts (text first,
	// then richer types). OutputModalities lists what it emits (text for chat).
	InputModalities  []Modality `json:"inputModalities,omitempty"`
	OutputModalities []Modality `json:"outputModalities,omitempty"`
	// ToolUse reports tool / function calling support.
	ToolUse bool `json:"toolUse,omitzero"`
	// StructuredOutput reports native structured-output / JSON-schema support.
	StructuredOutput bool `json:"structuredOutput,omitzero"`
}

// ModelPricing describes per-million-token pricing. The primary rate
// band; cache rates are zero when the provider doesn't price cache separately.
// Long-context models that reprice past a token threshold carry only their base
// band here — full banded pricing isn't surfaced on the wire.
type ModelPricing struct {
	InputUSDPerMillionTokens      float64 `json:"inputUsdPerMillionTokens,omitzero"`
	OutputUSDPerMillionTokens     float64 `json:"outputUsdPerMillionTokens,omitzero"`
	CacheReadUSDPerMillionTokens  float64 `json:"cacheReadUsdPerMillionTokens,omitzero"`
	CacheWriteUSDPerMillionTokens float64 `json:"cacheWriteUsdPerMillionTokens,omitzero"`
}
