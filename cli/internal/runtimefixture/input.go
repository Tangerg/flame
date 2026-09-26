package runtimefixture

import (
	"context"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	"github.com/Tangerg/flame/runtime/protocol"
)

// PrepareInput supplies deterministic fixture content. Real filesystem bytes
// and their restart behavior are exercised at the runtimebinding boundary.
func (*Runtime) PrepareInput(ctx context.Context, message agent.Message) ([]protocol.ContentBlock, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	if err := message.Validate(); err != nil {
		return nil, err
	}
	if len(message.Attachments) == 0 {
		return nil, nil
	}
	var input []protocol.ContentBlock
	if message.HasText() {
		input = append(input, protocol.ContentBlock{Type: protocol.ContentBlockText, Text: message.Text})
	}
	for _, attachment := range message.Attachments {
		block := protocol.ContentBlock{Type: attachment.Kind}
		if attachment.Kind == protocol.ContentBlockImage {
			block.Mime, block.Data = attachment.MimeType, "AA=="
		} else {
			block.Text = attachment.Name
		}
		input = append(input, block)
	}
	return input, nil
}
