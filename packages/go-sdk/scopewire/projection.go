// Package scopewire declares Scope values at Flame's portable JSON boundary.
// Scope retains semantic admission; these projections make byte, numeric and
// timestamp encodings explicit without adding another model of their behavior.
package scopewire

import (
	"encoding/json/jsontext"

	"github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/content"
	"github.com/Tangerg/scope/core/media"
	"github.com/Tangerg/scope/core/metadata"
	"github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/packages/go-sdk/instant"
)

type requestProjection struct {
	Messages   []messageProjection `json:"messages"`
	Tools      []tool.Definition   `json:"tools,omitempty"`
	ToolChoice *chat.ToolChoice    `json:"tool_choice,omitzero"`
	Options    optionsProjection   `json:"options,omitzero"`
}

type optionsProjection struct {
	Model            string               `json:"model,omitempty"`
	OutputFormat     *chat.OutputFormat   `json:"output_format,omitzero"`
	FrequencyPenalty *value.Number        `json:"frequency_penalty,omitzero"`
	MaxOutputTokens  *int64               `json:"max_output_tokens,omitzero"`
	PresencePenalty  *value.Number        `json:"presence_penalty,omitzero"`
	ReasoningEffort  chat.ReasoningEffort `json:"reasoning_effort,omitempty"`
	Stop             []string             `json:"stop,omitzero"`
	Temperature      *value.Number        `json:"temperature,omitzero"`
	TopK             *int64               `json:"top_k,omitzero"`
	TopP             *value.Number        `json:"top_p,omitzero"`
	Extensions       metadata.Extensions  `json:"extensions,omitzero"`
}

type messageProjection struct {
	Role     chat.Role        `json:"role"`
	Parts    []partProjection `json:"parts"`
	Metadata metadata.Map     `json:"metadata,omitzero"`
}

type partProjection struct {
	Kind           chat.PartKind         `json:"kind"`
	Text           string                `json:"text,omitempty"`
	Media          *mediaProjection      `json:"media,omitzero"`
	ReasoningState string                `json:"reasoning_state,omitempty"`
	ToolCall       *chat.ToolCall        `json:"tool_call,omitzero"`
	ToolResult     *toolResultProjection `json:"tool_result,omitzero"`
	Citations      []content.Citation    `json:"citations,omitempty"`
	Metadata       metadata.Map          `json:"metadata,omitzero"`
}

type mediaProjection struct {
	MIME     string                `json:"mime,omitzero"`
	Source   mediaSourceProjection `json:"source"`
	ID       string                `json:"id,omitempty"`
	Name     string                `json:"name,omitempty"`
	Metadata metadata.Map          `json:"metadata,omitzero"`
}

type mediaSourceProjection struct {
	Kind  media.SourceKind `json:"kind"`
	Bytes string           `json:"bytes,omitempty"`
	URI   string           `json:"uri,omitempty"`
	Ref   string           `json:"ref,omitempty"`
}

type blockProjection struct {
	Kind      content.Kind       `json:"kind"`
	Text      string             `json:"text,omitempty"`
	Media     *mediaProjection   `json:"media,omitzero"`
	Citations []content.Citation `json:"citations,omitempty"`
	Metadata  metadata.Map       `json:"metadata,omitzero"`
}

type toolOutputProjection struct {
	Content []blockProjection `json:"content,omitempty"`
	Details jsontext.Value    `json:"details,omitzero"`
}

type toolResultProjection struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Output  toolOutputProjection `json:"output"`
	IsError bool                 `json:"is_error,omitzero"`
}

type responseProjection struct {
	Output   *outputProjection           `json:"output,omitzero"`
	Metadata *responseMetadataProjection `json:"metadata,omitzero"`
}

type outputProjection struct {
	Message      *messageProjection   `json:"message,omitzero"`
	FinishReason chat.FinishReason    `json:"finish_reason,omitempty"`
	Metadata     *chat.OutputMetadata `json:"metadata,omitzero"`
}

type responseMetadataProjection struct {
	ID        string            `json:"id,omitempty"`
	Model     string            `json:"model,omitempty"`
	Usage     *chat.Usage       `json:"usage,omitzero"`
	CreatedAt instant.Timestamp `json:"created_at,omitzero"`
	Extra     metadata.Map      `json:"extra,omitzero"`
}

type responseDeltaProjection struct {
	Parts           []partDeltaProjection       `json:"parts,omitempty"`
	MessageMetadata metadata.Map                `json:"message_metadata,omitzero"`
	FinishReason    chat.FinishReason           `json:"finish_reason,omitempty"`
	OutputMetadata  *chat.OutputMetadata        `json:"output_metadata,omitzero"`
	Metadata        *responseMetadataProjection `json:"metadata,omitzero"`
}

type partDeltaProjection struct {
	Kind           chat.PartDeltaKind  `json:"kind"`
	Text           string              `json:"text,omitempty"`
	Media          *mediaProjection    `json:"media,omitzero"`
	ReasoningState string              `json:"reasoning_state,omitempty"`
	ToolCall       *chat.ToolCallDelta `json:"tool_call,omitzero"`
	Citation       *content.Citation   `json:"citation,omitzero"`
	Metadata       metadata.Map        `json:"metadata,omitzero"`
}
