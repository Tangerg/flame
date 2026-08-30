package conversation

import (
	"errors"
	"fmt"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/scope/core/chat"
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
	if value == "" {
		return ToolCallIdentity{}, fmt.Errorf("%w: empty", ErrToolCallIdentity)
	}
	if !utf8.ValidString(value) {
		return ToolCallIdentity{}, fmt.Errorf("%w: invalid UTF-8", ErrToolCallIdentity)
	}
	if characters := utf8.RuneCountInString(value); characters > MaximumToolCallIdentityCharacters {
		return ToolCallIdentity{}, fmt.Errorf(
			"%w: %d characters exceeds %d",
			ErrToolCallIdentity,
			characters,
			MaximumToolCallIdentityCharacters,
		)
	}
	for _, character := range value {
		if unicode.IsSpace(character) || !unicode.IsPrint(character) {
			return ToolCallIdentity{}, fmt.Errorf("%w: contains whitespace or a non-printing character", ErrToolCallIdentity)
		}
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
