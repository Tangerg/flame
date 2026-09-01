package testsupport

import (
	"errors"
	"fmt"
	"iter"

	"github.com/Tangerg/scope/core/chat"
)

// ResponseDelta projects a complete fixture response into one terminal stream
// increment. Production providers build their own transport deltas; this helper
// keeps Runtime tests on the same accumulator contract without duplicating a
// synthetic provider in every package.
func ResponseDelta(response *chat.Response) (*chat.ResponseDelta, error) {
	if response == nil {
		return nil, errors.New("testsupport: nil chat response")
	}
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("testsupport: invalid chat response: %w", err)
	}
	delta := &chat.ResponseDelta{
		FinishReason:   response.Output.FinishReason,
		OutputMetadata: response.Output.Metadata,
		Metadata:       response.Metadata,
	}
	if response.Output.Message == nil {
		return delta, nil
	}
	delta.MessageMetadata = response.Output.Message.Metadata
	for _, part := range response.Output.Message.Parts {
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
	}
	if err := delta.Validate(); err != nil {
		return nil, fmt.Errorf("testsupport: invalid chat response delta: %w", err)
	}
	return delta, nil
}

// StreamResponse converts one complete fixture result into one lazy terminal
// delta or one terminal error.
func StreamResponse(response *chat.Response, responseErr error) iter.Seq2[*chat.ResponseDelta, error] {
	return func(yield func(*chat.ResponseDelta, error) bool) {
		if responseErr != nil {
			yield(nil, responseErr)
			return
		}
		delta, err := ResponseDelta(response)
		yield(delta, err)
	}
}
