package agentexec

import (
	"encoding/json"
	"testing"

	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/metadata"
)

func TestFrozenInstructionsPreserveExactMetadataNumbers(t *testing.T) {
	for _, partMetadata := range []bool{false, true} {
		frozen := chat.NewSystemMessage("instructions")
		values := metadata.Map{"source": json.RawMessage(`{"id":9007199254740992,"revision":1}`)}
		if partMetadata {
			frozen.Parts[0].Metadata = values
		} else {
			frozen.Metadata = values
		}
		for _, test := range []struct {
			value string
			equal bool
		}{
			{value: `{ "revision": 1, "id": 9007199254740992 }`, equal: true},
			{value: `{"id":9007199254740993,"revision":1}`, equal: false},
		} {
			candidate := frozen.Clone()
			if partMetadata {
				candidate.Parts[0].Metadata["source"] = json.RawMessage(test.value)
			} else {
				candidate.Metadata["source"] = json.RawMessage(test.value)
			}
			equal, err := sameInteractionMessages([]chat.Message{candidate}, []chat.Message{frozen})
			if err != nil || equal != test.equal {
				t.Fatalf("metadata=%s part=%t equality=%t want=%t error=%v", test.value, partMetadata, equal, test.equal, err)
			}
		}
	}
}

func TestFrozenInstructionsPreserveCitations(t *testing.T) {
	frozen := chat.NewSystemMessage("Follow the cited workspace policy.")
	frozen.Parts[0].Citations = []chat.Citation{{
		Source: chat.CitationSource{Kind: chat.CitationSourceReference, Value: "workspace-policy"},
		Title:  "Policy", Quote: "Require approval before publishing.",
	}}
	for name, mutate := range map[string]func(*chat.Message){
		"removed": func(message *chat.Message) { message.Parts[0].Citations = nil },
		"source":  func(message *chat.Message) { message.Parts[0].Citations[0].Source.Value = "other-policy" },
		"title":   func(message *chat.Message) { message.Parts[0].Citations[0].Title = "Other policy" },
		"quote":   func(message *chat.Message) { message.Parts[0].Citations[0].Quote = "Publish immediately." },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := frozen.Clone()
			mutate(&candidate)
			equal, err := sameInteractionMessages([]chat.Message{candidate}, []chat.Message{frozen})
			if err != nil || equal {
				t.Fatalf("changed frozen citation: equal=%t error=%v", equal, err)
			}
		})
	}
}

func TestTrailingUserMessageCountPreservesEveryMessageInOneSteerSignal(t *testing.T) {
	messages := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("delegated task")),
		chat.NewAssistantMessage(chat.NewTextPart("working")),
		chat.NewUserMessage(chat.NewTextPart("first steer message")),
		chat.NewUserMessage(chat.NewTextPart("second steer message")),
	}

	if got := trailingUserMessageCount(messages); got != 2 {
		t.Fatalf("trailing User messages = %d, want 2", got)
	}
}
