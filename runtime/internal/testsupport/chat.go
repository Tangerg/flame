package testsupport

import (
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/scope/core/chat"
)

func responseDelta(response *chat.Response) (*chat.ResponseDelta, error) {
	if response == nil {
		return nil, errors.New("testsupport: nil chat response")
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("testsupport: invalid chat response: %w", err)
	}
	delta := &chat.ResponseDelta{
		FinishReason: response.Output.FinishReason,
		Metadata:     response.Metadata,
	}
	if response.Output.Message == nil {
		return delta, nil
	}
	delta.MessageMetadata = response.Output.Message.Metadata
	for index, part := range response.Output.Message.Parts {
		firstDelta := len(delta.Parts)
		switch part.Kind {
		case chat.PartText:
			delta.Parts = append(delta.Parts, chat.NewTextDelta(part.Text))
		case chat.PartMedia:
			delta.Parts = append(delta.Parts, chat.NewMediaDelta(part.Media))
		case chat.PartReasoning:
			delta.Parts = append(delta.Parts, chat.NewReasoningDelta(part.Text, part.ReasoningState))
		case chat.PartToolCall:
			delta.Parts = append(delta.Parts, chat.NewToolCallDelta(chat.ToolCallDelta{
				ID: part.ToolCall.ID, Name: part.ToolCall.Name, Arguments: part.ToolCall.Arguments,
			}))
		case chat.PartRefusal:
			delta.Parts = append(delta.Parts, chat.NewRefusalDelta(part.Text))
		default:
			return nil, fmt.Errorf("testsupport: response part kind %q cannot be streamed", part.Kind)
		}
		for _, citation := range part.Citations {
			delta.Parts = append(delta.Parts, chat.NewCitationDelta(citation))
		}
		for i := firstDelta; i < len(delta.Parts); i++ {
			delta.Parts[i].Index = index
		}
	}
	if err := delta.Validate(); err != nil {
		return nil, fmt.Errorf("testsupport: invalid chat response delta: %w", err)
	}
	return delta, nil
}

func StreamResponse(response *chat.Response, responseErr error) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		if responseErr != nil {
			yield(nil, responseErr)
			return
		}
		delta, err := responseDelta(response)
		yield(delta, err)
	}
}
