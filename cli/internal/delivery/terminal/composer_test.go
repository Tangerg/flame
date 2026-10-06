package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/oolong/components/headless"
)

func TestPromptHistoryRestoresAttachmentsAndDraft(t *testing.T) {
	file := prompt.Attachment{ID: "att_1", Kind: protocol.ContentBlockText, Name: "main.go", Path: "/tmp/main.go", Size: 10}
	var history promptHistory
	history.Add(prompt.Message{Text: "inspect", Attachments: []prompt.Attachment{file}})
	got, ok := history.Back(prompt.Message{Text: "draft"})
	if !ok || got.Text != "inspect" || len(got.Attachments) != 1 || got.Attachments[0].ID != file.ID {
		t.Fatalf("back = %+v, %v", got, ok)
	}
	got.Attachments[0].Name = "mutated"
	draft, ok := history.Forward()
	if !ok || draft.Text != "draft" {
		t.Fatalf("forward = %+v, %v", draft, ok)
	}
	again, _ := history.Back(prompt.Message{})
	if again.Attachments[0].Name != "main.go" {
		t.Fatalf("history leaked caller mutation: %+v", again)
	}
}

func TestPromptHistoryDropsConsecutiveDuplicates(t *testing.T) {
	var history promptHistory
	history.Add(prompt.Message{Text: "same"})
	history.Add(prompt.Message{Text: "same"})
	if len(history.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(history.entries))
	}
}

func TestPromptHistoryOwnsAndEnforcesItsRetentionCapacity(t *testing.T) {
	var history promptHistory
	for index := range promptHistoryCapacity + 5 {
		history.Add(prompt.Message{Text: fmt.Sprintf("prompt %d", index)})
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

func TestCommittedComposerCannotUndoIntoReleasedAttachmentPayloads(t *testing.T) {
	a := &app{attachmentElements: make(map[uint64]prompt.Attachment)}
	editor := a.composer.Editor()
	element := editor.InsertElement(fileElement, "@design.md")
	a.attachmentElements[element.ID] = prompt.Attachment{Name: "design.md"}
	editor.Insert(" inspect this")
	a.clearComposer()
	editor.Do(headless.Undo)
	if !editor.Empty() || len(editor.Elements()) != 0 {
		t.Fatal("committed draft can resurrect text or released attachments")
	}
}

func TestRejectedSteerAttachmentsNeverReplaceAnUnreadableDraft(t *testing.T) {
	a := &app{attachmentElements: make(map[uint64]prompt.Attachment), closed: true}
	editor := a.composer.Editor()
	editor.InsertElement(fileElement, "@orphan.md")
	editor.Insert(" keep this text")
	a.restoreSteerAttachments([]prompt.Attachment{{Name: "rejected.md"}})
	if text := editor.Text(); !strings.Contains(text, "keep this text") || !strings.Contains(text, "@rejected.md") {
		t.Fatalf("composer = %q, want the user's text kept and the rejected attachment inserted", text)
	}
}
