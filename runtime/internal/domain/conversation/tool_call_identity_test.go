package conversation

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestProviderToolCallIdentityIsExactAndBounded(t *testing.T) {
	want := strings.Repeat("界", MaximumToolCallIdentityCharacters)
	identity, err := NewToolCallIdentity(want)
	if err != nil {
		t.Fatalf("NewToolCallIdentity boundary: %v", err)
	}
	if identity.String() != want {
		t.Fatalf("identity = %q, want exact input", identity.String())
	}

	invalid := []string{
		"",
		" call_1",
		"call_1 ",
		"call\n1",
		"call\u200b1",
		string([]byte{0xff}),
		strings.Repeat("界", MaximumToolCallIdentityCharacters+1),
	}
	for _, value := range invalid {
		if _, err := NewToolCallIdentity(value); !errors.Is(err, ErrToolCallIdentity) {
			t.Errorf("NewToolCallIdentity(%q) error = %v", value, err)
		}
	}

	if _, present, err := ParseOptionalToolCallIdentity(""); err != nil || present {
		t.Fatalf("optional empty identity = present:%t error:%v", present, err)
	}
}

func TestConversationRejectsInvalidProviderToolCallIdentity(t *testing.T) {
	message := chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
		ID: "call\u200b1", Name: "inspect", Arguments: `{}`,
	}))
	if _, err := New([]chat.Message{message}); !errors.Is(err, ErrToolCallIdentity) {
		t.Fatalf("New error = %v, want ErrToolCallIdentity", err)
	}
}
