package model

import (
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

const (
	// The media placeholders are the smallest valid values for their Source
	// variants. A provider prices image/audio/document content by modality, not
	// by the size of its transport encoding, so raw payload bytes must not be
	// mistaken for text tokens in the provider-neutral estimate.
	mediaURIPlaceholder       = "data:,x"
	mediaReferencePlaceholder = "x"
)

// EstimateRequestTokens measures the complete provider-neutral request:
// messages of every Part kind, Tool definitions, Options, metadata, provider
// replay signatures, Tool identities, arguments, and results. It intentionally
// does not use RenderTranscript: that lossy auxiliary-model view is a different
// artifact and omits model-visible protocol state.
func EstimateRequestTokens(source *chat.Request) (int, error) {
	if source == nil {
		return 0, errors.New("model text: request is required")
	}
	request := *source

	if err := request.Validate(); err != nil {
		return 0, err
	}
	request.Messages = modelContextEstimateMessages(source.Messages)
	request.Tools = make([]chat.ToolDefinition, len(source.Tools))
	for index, tool := range source.Tools {
		request.Tools[index] = tool.Clone()
	}
	request.Options = source.Options.Clone()
	payload, err := json.Marshal(&request)
	if err != nil {
		return 0, err
	}
	return EstimateTextTokens(string(payload)), nil
}

func modelContextEstimateMessages(messages []chat.Message) []chat.Message {
	normalized := make([]chat.Message, len(messages))
	for messageIndex := range messages {
		message := messages[messageIndex]
		normalized[messageIndex] = chat.Message{
			Role:     message.Role,
			Parts:    make([]chat.Part, len(message.Parts)),
			Metadata: message.Metadata.Clone(),
		}
		for partIndex := range message.Parts {
			part := message.Parts[partIndex]
			value := part.Media
			result := part.ToolResult
			part.Media = nil
			part.ToolResult = nil
			normalized[messageIndex].Parts[partIndex] = part.Clone()
			normalized[messageIndex].Parts[partIndex].Media = normalizedMedia(value)
			normalized[messageIndex].Parts[partIndex].ToolResult = modelContextEstimateToolResult(result)
		}
	}
	return normalized
}

func normalizedMedia(value *media.Media) *media.Media {
	if value == nil {
		return nil
	}
	normalized := *value
	normalized.Source.Bytes = nil
	normalized.Metadata = value.Metadata.Clone()
	switch normalized.Source.Kind {
	case media.SourceBytes:
		normalized.Source.Bytes = []byte{0}
	case media.SourceURI:
		normalized.Source.URI = mediaURIPlaceholder
	case media.SourceReference:
		normalized.Source.Ref = mediaReferencePlaceholder
	}
	return &normalized
}

func modelContextEstimateToolResult(result *chat.ToolResult) *chat.ToolResult {
	if result == nil {
		return nil
	}
	// Replace media before cloning so the estimate never copies inline payloads.
	withoutContent := *result
	// Content supersedes Details in the provider projection.
	if len(result.Output.Content) > 0 {
		withoutContent.Output.Details = nil
	}
	withoutContent.Output.Content = nil
	normalized := withoutContent.Clone()
	normalized.Output.Content = make([]chat.ToolContent, len(result.Output.Content))
	for index, content := range result.Output.Content {
		value := content.Media
		content.Media = nil
		normalized.Output.Content[index] = content.Clone()
		normalized.Output.Content[index].Media = normalizedMedia(value)
	}
	return &normalized
}
