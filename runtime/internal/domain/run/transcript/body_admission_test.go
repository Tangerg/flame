package transcript_test

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func TestItemRestorationRejectsPointerBodies(t *testing.T) {
	for _, test := range []struct {
		name string
		body transcript.Body
	}{
		{"mutable_reasoning", &transcript.ReasoningBody{Text: "captured reasoning"}},
		{"nil_user_message", (*transcript.UserMessageBody)(nil)},
		{"nil_agent_message", (*transcript.AgentMessageBody)(nil)},
		{"nil_reasoning", (*transcript.ReasoningBody)(nil)},
		{"nil_question", (*transcript.QuestionBody)(nil)},
		{"nil_tool_call", (*transcript.ToolCallBody)(nil)},
		{"nil_compaction", (*transcript.CompactionBody)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := transcript.RestoreItem(transcript.ItemSnapshot{
				Identity: itemIdentity(), Status: transcript.ItemCompleted, Body: test.body,
			})
			if err == nil {
				t.Fatal("restoration accepted a pointer body")
			}
		})
	}
}
