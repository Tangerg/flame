package model

import (
	json "encoding/json/v2"
	"errors"

	corecontent "github.com/Tangerg/scope/core/content"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/media"
)

const (
	// Providers price media by modality, not by transport encoding size, so raw
	// payload bytes must not be mistaken for text tokens.
	mediaURIPlaceholder       = "data:,x"
	mediaReferencePlaceholder = "x"
)

// RenderTranscript is deliberately not used: that lossy auxiliary-model view omits
// model-visible protocol state.
func EstimateRequestTokens(source *chat.Request) (int, error) {
	if source == nil {
		return 0, errors.New("model text: request is required")
	}
	request := *source

	if err := request.Validate(); err != nil {
		return 0, err
	}
	request.Messages = modelContextEstimateMessages(source.Messages)
	request.Tools = make([]toolcontract.Definition, len(source.Tools))
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
	if len(result.Output.Content) > 0 {
		withoutContent.Output.Details = nil
	}
	withoutContent.Output.Content = nil
	normalized := withoutContent.Clone()
	normalized.Output.Content = make([]corecontent.Block, len(result.Output.Content))
	for index, content := range result.Output.Content {
		value := content.Media
		content.Media = nil
		normalized.Output.Content[index] = content.Clone()
		normalized.Output.Content[index].Media = normalizedMedia(value)
	}
	return &normalized
}
