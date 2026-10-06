package terminal

import (
	"strings"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestInterruptReviewRecordsEditsAndCommitsInRuntimeOrder(t *testing.T) {
	approval := conversation.Approval{
		RunID: "run_1", ItemID: "approval", Title: "Run command", Rememberable: true,
		Tool: &conversation.ToolCall{Kind: conversation.ToolShell, Name: "shell", Command: "go test ./...", Status: conversation.ToolRunning},
	}
	question := conversation.Question{
		RunID: "run_1", ItemID: "question", Title: "Choose target",
		Fields: []conversation.QuestionField{{Prompt: "Target", Kind: conversation.QuestionSingle, Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}}}},
	}
	review, err := newInterruptReview([]conversation.Interrupt{approval, question})
	if err != nil {
		t.Fatal(err)
	}
	if recordErr := review.Record(conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberSession}); recordErr != nil {
		t.Fatal(recordErr)
	}
	if !review.Advance() {
		t.Fatal("review did not advance to the question")
	}
	if recordErr := review.Record(conversation.QuestionAnswer{Values: [][]string{{"linux"}}}); recordErr != nil {
		t.Fatal(recordErr)
	}
	if review.Advance() || !review.Reviewing() {
		t.Fatal("review did not enter final review")
	}
	if !review.Back() {
		t.Fatal("review did not return to the final item")
	}
	if recordErr := review.Record(conversation.QuestionAnswer{Values: [][]string{{"darwin"}}}); recordErr != nil {
		t.Fatal(recordErr)
	}
	review.Advance()
	responses, err := review.Responses()
	if err != nil {
		t.Fatal(err)
	}
	if len(responses) != 2 || responses[0].ItemID != "approval" || responses[1].ItemID != "question" {
		t.Fatalf("responses = %+v", responses)
	}
	answer, ok := responses[1].Answer.(conversation.QuestionAnswer)
	if !ok || answer.Values[0][0] != "darwin" {
		t.Fatalf("edited question answer = %#v", responses[1].Answer)
	}
}

func TestInterruptReviewRejectsInvalidAnswersAndIncompleteCommit(t *testing.T) {
	approval := conversation.Approval{
		RunID: "run_1", ItemID: "approval", Title: "Read file",
		Tool: &conversation.ToolCall{Kind: conversation.ToolRead, Name: "read", Path: "README.md", Status: conversation.ToolRunning},
	}
	review, err := newInterruptReview([]conversation.Interrupt{approval})
	if err != nil {
		t.Fatal(err)
	}
	if err := review.Record(conversation.QuestionAnswer{}); err == nil {
		t.Fatal("invalid answer was accepted")
	}
	if _, err := review.Responses(); err == nil {
		t.Fatal("incomplete review was committed")
	}
}

func TestInterruptReviewRestoresACommittedBatchWithoutSharingAnswers(t *testing.T) {
	approval := conversation.Approval{
		RunID: "run_1", ItemID: "approval", Title: "Run command", Rememberable: true,
		Tool: &conversation.ToolCall{Kind: conversation.ToolShell, Name: "shell", Command: "go test ./...", Status: conversation.ToolRunning},
	}
	question := conversation.Question{
		RunID: "run_1", ItemID: "question", Title: "Choose target",
		Fields: []conversation.QuestionField{{
			Prompt: "Target", Kind: conversation.QuestionMulti,
			Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}},
		}},
	}
	responses := []conversation.InterruptAnswer{
		{ItemID: approval.ItemID, Answer: conversation.ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberSession}},
		{ItemID: question.ItemID, Answer: conversation.QuestionAnswer{Values: [][]string{{"linux", "darwin"}}}},
	}
	review, err := restoreInterruptReview([]conversation.Interrupt{approval, question}, responses)
	if err != nil {
		t.Fatal(err)
	}
	if !review.Reviewing() {
		t.Fatal("restored review is not at its committed summary")
	}
	responses[1].Answer.(conversation.QuestionAnswer).Values[0][0] = "mutated"
	committed, err := review.Responses()
	if err != nil {
		t.Fatal(err)
	}
	answer := committed[1].Answer.(conversation.QuestionAnswer)
	if answer.Values[0][0] != "linux" {
		t.Fatalf("restored answer shares caller storage: %+v", answer.Values)
	}
}

func TestInterruptSummaryDisclosesEditedApprovalArguments(t *testing.T) {
	t.Parallel()
	approval := conversation.Approval{
		RunID: "run_1", ItemID: "approval", Title: "Run command",
		Tool: &conversation.ToolCall{Kind: conversation.ToolShell, Name: "shell", Status: conversation.ToolRunning},
	}
	override, err := conversation.ParseToolArgumentOverride([]byte(`{"command":"echo safe"}`))
	if err != nil {
		t.Fatal(err)
	}
	review, err := restoreInterruptReview(
		[]conversation.Interrupt{approval},
		[]conversation.InterruptAnswer{{
			ItemID: approval.ItemID,
			Answer: conversation.ApprovalAnswer{
				Decision: protocol.ApprovalApprove, ArgumentOverride: override,
			},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	got := summarizeInterrupt(review.items[0], review.answers[0])
	for _, want := range []string{"with edited arguments", `{"command":"echo safe"}`} {
		if !strings.Contains(got, want) {
			t.Fatalf("interrupt summary %q omits %q", got, want)
		}
	}
}

func TestInterruptSummaryDisclosesRememberedDenial(t *testing.T) {
	t.Parallel()
	approval := conversation.Approval{Title: "Delete generated file"}
	answer := conversation.ApprovalAnswer{
		Decision: protocol.ApprovalDeny, Remember: protocol.RememberProject, Reason: "preserve fixtures",
	}
	got := summarizeInterrupt(approval, answer)
	for _, want := range []string{"deny for project", "preserve fixtures"} {
		if !strings.Contains(got, want) {
			t.Fatalf("interrupt summary %q omits %q", got, want)
		}
	}
}
