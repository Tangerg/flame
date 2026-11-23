package protocol

type UtilityRole struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

type EmbeddingRole struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
}

type ListModelsRequest struct {
	Provider string `json:"provider"`
}

type Model struct {
	ID              string             `json:"id"`
	Provider        string             `json:"provider"`
	DisplayName     string             `json:"displayName,omitempty"`
	TokenLimits     *ModelTokenLimits  `json:"tokenLimits,omitzero"`
	KnowledgeCutoff string             `json:"knowledgeCutoff,omitempty"`
	Deprecated      bool               `json:"deprecated,omitzero"`
	Default         bool               `json:"default,omitzero"`
	Capabilities    *ModelCapabilities `json:"capabilities,omitzero"`
	Pricing         []ModelPriceBand   `json:"pricing,omitempty"`
}

// The three maxima are independent facts, not simultaneously attainable quotas: an
// output maximum may exceed the input context window.
type ModelTokenLimits struct {
	ContextWindow   *int64 `json:"contextWindow,omitzero"`
	MaxInputTokens  *int64 `json:"maxInputTokens,omitzero"`
	MaxOutputTokens *int64 `json:"maxOutputTokens,omitzero"`
}

//flame:enum
type Modality string

const (
	ModalityText  Modality = "text"
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
	ModalityPDF   Modality = "pdf"
)

type ModelCapabilities struct {
	Reasoning             bool       `json:"reasoning,omitzero"`
	ReasoningLevels       []string   `json:"reasoningLevels,omitempty"`
	ReasoningDefaultLevel string     `json:"reasoningDefaultLevel,omitempty"`
	Multimodal            bool       `json:"multimodal,omitzero"`
	InputModalities       []Modality `json:"inputModalities,omitempty"`
	OutputModalities      []Modality `json:"outputModalities,omitempty"`
	ToolUse               bool       `json:"toolUse,omitzero"`
	StructuredOutput      bool       `json:"structuredOutput,omitzero"`
}

type ModelPriceBand struct {
	AboveInputTokens              int64   `json:"aboveInputTokens,omitzero"`
	InputUSDPerMillionTokens      float64 `json:"inputUsdPerMillionTokens,omitzero"`
	OutputUSDPerMillionTokens     float64 `json:"outputUsdPerMillionTokens,omitzero"`
	CacheReadUSDPerMillionTokens  float64 `json:"cacheReadUsdPerMillionTokens,omitzero"`
	CacheWriteUSDPerMillionTokens float64 `json:"cacheWriteUsdPerMillionTokens,omitzero"`
}
