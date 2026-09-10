package conversation

import (
	"errors"
	"fmt"

	"github.com/Tangerg/scope/core/chat"

	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

const MaximumToolCallIdentityCharacters = 512

var ErrToolCallIdentity = errors.New("conversation: invalid provider ToolCall identity")

// ToolCallIdentity is the exact provider-neutral correlation key in a model
// ToolCall/ToolResult pair. It is distinct from the executor Effect ID that
// owns one attempt to execute that call.
type ToolCallIdentity struct {
	value string
}

func NewToolCallIdentity(value string) (ToolCallIdentity, error) {
	if err := runtimeidentity.ValidateText(value, MaximumToolCallIdentityCharacters); err != nil {
		return ToolCallIdentity{}, fmt.Errorf("%w: %v", ErrToolCallIdentity, err)
	}
	return ToolCallIdentity{value: value}, nil
}

func ParseOptionalToolCallIdentity(value string) (ToolCallIdentity, bool, error) {
	if value == "" {
		return ToolCallIdentity{}, false, nil
	}
	identity, err := NewToolCallIdentity(value)
	return identity, err == nil, err
}

func (i ToolCallIdentity) String() string { return i.value }

// ValidateMessageIdentities proves every provider correlation identity carried
// by a conversation message.
func ValidateMessageIdentities(message chat.Message) error {
	for index, part := range message.Parts {
		var identity string
		var kind string
		switch {
		case part.ToolCall != nil:
			identity, kind = part.ToolCall.ID, "ToolCall"
		case part.ToolResult != nil:
			identity, kind = part.ToolResult.ID, "ToolResult"
		default:
			continue
		}
		if _, err := NewToolCallIdentity(identity); err != nil {
			return fmt.Errorf("message part %d %s: %w", index, kind, err)
		}
	}
	return nil
}
