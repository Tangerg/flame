package coordination

import "context"

// The workflow consumes a transcript already captured by the Host. Its fixture
// is a value at that boundary, not a second message-to-transcript renderer.
type capturedHistory struct{ conversation Conversation }

func (h capturedHistory) Read(context.Context, string) (Conversation, error) {
	return h.conversation, nil
}
