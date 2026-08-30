package terminal

import (
	"fmt"
	"testing"

	"github.com/Tangerg/flame/cli/internal/agent"
)

func TestPromptHistoryRestoresAttachmentsAndDraft(t *testing.T) {
	file := agent.Attachment{ID: "att_1", Kind: agent.AttachmentText, Name: "main.go", Path: "/tmp/main.go", Size: 10}
	var history promptHistory
	history.Add(agent.Message{Text: "inspect", Attachments: []agent.Attachment{file}})
	got, ok := history.Back(agent.Message{Text: "draft"})
	if !ok || got.Text != "inspect" || len(got.Attachments) != 1 || got.Attachments[0].ID != file.ID {
		t.Fatalf("back = %+v, %v", got, ok)
	}
	got.Attachments[0].Name = "mutated"
	draft, ok := history.Forward()
	if !ok || draft.Text != "draft" {
		t.Fatalf("forward = %+v, %v", draft, ok)
	}
	again, _ := history.Back(agent.Message{})
	if again.Attachments[0].Name != "main.go" {
		t.Fatalf("history leaked caller mutation: %+v", again)
	}
}

func TestPromptHistoryDropsConsecutiveDuplicates(t *testing.T) {
	var history promptHistory
	history.Add(agent.Message{Text: "same"})
	history.Add(agent.Message{Text: "same"})
	if len(history.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(history.entries))
	}
}

func TestPromptHistoryOwnsAndEnforcesItsRetentionCapacity(t *testing.T) {
	var history promptHistory
	for index := range promptHistoryCapacity + 5 {
		history.Add(agent.Message{Text: fmt.Sprintf("prompt %d", index)})
	}
	if len(history.entries) != promptHistoryCapacity {
		t.Fatalf("entries = %d, want %d", len(history.entries), promptHistoryCapacity)
	}
	if got := history.entries[0].Text; got != "prompt 5" {
		t.Fatalf("oldest retained prompt = %q, want prompt 5", got)
	}
	if got := history.entries[len(history.entries)-1].Text; got != "prompt 1004" {
		t.Fatalf("newest retained prompt = %q, want prompt 1004", got)
	}
}
