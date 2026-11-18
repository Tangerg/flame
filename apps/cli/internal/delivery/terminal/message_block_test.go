package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"

	"github.com/Tangerg/oolong/ui/kit"
)

func TestUserPresenterKeepsAttachmentsInsideTheMessageSurface(t *testing.T) {
	rendered := presentUser(blockPresentation{Theme: kit.Dark()}, conversation.Block{
		Kind:        conversation.BlockUser,
		Text:        "review this",
		Attachments: []authoring.Attachment{{Name: "design.md", MimeType: "text/markdown"}},
	})
	if len(rendered) != 1 {
		t.Fatalf("presented blocks = %d, want 1", len(rendered))
	}
	message, ok := rendered[0].(*userMessageBlock)
	if !ok || !strings.Contains(message.message.Body, "@design.md · text/markdown") {
		t.Fatalf("presented user message = %#v", rendered[0])
	}
}

func TestQuestionPresenterShowsAcceptedTranscriptAnswers(t *testing.T) {
	t.Parallel()

	question := conversation.Question{
		Title: "Target", Fields: []conversation.QuestionField{{Prompt: "Which platform?"}},
		Answers: [][]string{{"linux"}},
	}
	rendered := presentQuestion(blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode()}, conversation.Block{Question: &question})
	if len(rendered) != 1 {
		t.Fatalf("presented blocks = %d, want 1", len(rendered))
	}
	message, ok := rendered[0].(*questionBlock)
	if !ok || !strings.Contains(message.message.Body, "answer · linux") {
		t.Fatalf("presented question = %#v", rendered[0])
	}
}

func TestPendingQuestionPresenterHasNoVisibleInterruptSurface(t *testing.T) {
	t.Parallel()
	question := conversation.Question{
		RunID: "run_1", ItemID: "question_1", Title: "Target",
		Fields: []conversation.QuestionField{{Prompt: "Which platform?", Kind: conversation.QuestionText}},
	}
	rendered := presentQuestion(blockPresentation{Theme: kit.Dark(), Glyphs: kit.Unicode()}, conversation.Block{Question: &question})
	block, ok := rendered[0].(*questionBlock)
	if !ok {
		t.Fatalf("presented question = %T", rendered[0])
	}
	if height := block.HeightForWidth(40); height != 0 || len(block.Rows(40)) != 0 {
		t.Fatalf("pending question occupies %d rows with copyable content %+v", height, block.Rows(40))
	}
}
