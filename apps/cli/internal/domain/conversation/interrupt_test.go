package conversation

import (
	"errors"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
)

func TestQuestionAnswerMatchesThePublicRuntimeContract(t *testing.T) {
	options := []protocol.QuestionOption{{Label: "a"}, {Label: "b"}}
	for _, field := range []QuestionField{
		{Prompt: "Context", Kind: QuestionText},
		{Prompt: "Platform", Kind: QuestionSingle, Options: options},
		{Prompt: "Platform", Kind: QuestionSingle, Options: options, AllowCustom: true},
		{Prompt: "Checks", Kind: QuestionMulti, Options: options},
		{Prompt: "Checks", Kind: QuestionMulti, Options: options, AllowCustom: true},
	} {
		question := Question{RunID: "run_1", ItemID: "item_1", Title: "Contract", Fields: []QuestionField{{Prompt: "First", Kind: QuestionText}, field}}
		contract, err := question.AnswerContract()
		if err != nil {
			t.Fatal(err)
		}
		for _, values := range [][]string{nil, {}, {"a"}, {"a", "b"}, {"custom, one"}, {"a", "custom, one"}, {"one", "two"}, {"a", "a"}, {"custom", "custom"}, {""}, {" "}, {" a"}, {"a "}, {" context "}} {
			answers := [][]string{{}, values}
			want := contract.Validate(answers)
			got := ValidateAnswer(question, QuestionAnswer{Values: answers})
			if (got == nil) != (want == nil) {
				t.Fatalf("%s custom=%t values=%q: CLI=%v, Runtime=%v", field.Kind, field.AllowCustom, values, got, want)
			}
			if want != nil {
				local, ok := errors.AsType[*questioncontract.FieldError](got)
				if !ok || local.Index != 1 {
					t.Fatalf("field identity = %v, want index 1", got)
				}
			}
			accepted := question.Clone()
			accepted.Answers = CloneAnswers(answers)
			if err := accepted.Validate(); (err == nil) != (want == nil) {
				t.Fatalf("accepted projection values=%q: %v, Runtime=%v", values, err, want)
			}
		}
	}
}

func TestQuestionAnswerUsesFieldOrder(t *testing.T) {
	question := Question{RunID: "run_1", ItemID: "q_1", Title: "Configuration", Fields: []QuestionField{
		{Prompt: "Name", Kind: QuestionText},
		{Prompt: "Targets", Kind: QuestionMulti, Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}}},
	}}
	answer := QuestionAnswer{Values: [][]string{{"flame"}, {"linux", "darwin"}}}
	if err := ValidateAnswer(question, answer); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAnswer(question, QuestionAnswer{Values: [][]string{{"flame"}}}); err == nil {
		t.Fatal("partial answer set was accepted")
	}
}

func TestQuestionProjectionPreservesExplicitSkippedAnswers(t *testing.T) {
	question := Question{RunID: "run_1", ItemID: "q_1", Title: "Skipped", Fields: []QuestionField{
		{Prompt: "Optional context", Kind: QuestionText},
	}, Answers: [][]string{{}}}
	if !question.Answered() {
		t.Fatal("explicit skip became an unanswered question")
	}
	if err := question.Validate(); err != nil {
		t.Fatalf("accepted skip was rejected: %v", err)
	}
	copy := question.Clone()
	if !copy.Answered() || len(copy.Answers) != 1 || len(copy.Answers[0]) != 0 {
		t.Fatal("cloning lost the accepted skip")
	}
}

func TestQuestionAnswerRejectsInvalidCustomChoiceValues(t *testing.T) {
	question := Question{RunID: "run_1", ItemID: "q_1", Title: "Choices", Fields: []QuestionField{
		{Prompt: "Targets", Kind: QuestionMulti, AllowCustom: true, Options: []protocol.QuestionOption{{Label: "linux"}, {Label: "darwin"}}},
	}}
	for _, values := range [][]string{{"custom one", "custom two"}, {" "}, {" linux"}, {"custom "}} {
		if err := ValidateAnswer(question, QuestionAnswer{Values: [][]string{values}}); err == nil {
			t.Errorf("invalid choice answer was accepted: %q", values)
		}
	}
}

func TestQuestionProjectionOwnsItsAcceptedSnapshotCopy(t *testing.T) {
	t.Parallel()
	answers := [][]string{{"linux"}}
	question := Question{RunID: "run_1", ItemID: "q_1", Title: "Target", Fields: []QuestionField{{Prompt: "Name", Kind: QuestionText}}, Answers: answers}
	if err := question.Validate(); err != nil {
		t.Fatal(err)
	}
	snapshot := question.Clone()
	answers[0][0] = "changed"
	if !snapshot.Answered() || snapshot.Answers[0][0] != "linux" {
		t.Fatal("snapshot did not retain the original accepted fact")
	}
}

func TestApprovalHonorsRememberable(t *testing.T) {
	approval := runningApproval("a_1", "shell")
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberProject}); err == nil {
		t.Fatal("unrememberable approval was remembered")
	}
	approval.Rememberable = true
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: protocol.ApprovalApprove, Remember: protocol.RememberProject}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateAnswer(approval, ApprovalAnswer{Decision: protocol.ApprovalDeny, Remember: protocol.RememberSession}); err != nil {
		t.Fatalf("remembered denial was rejected: %v", err)
	}
}

func TestApprovalAnswerOwnsOnlyApprovedArgumentOverrides(t *testing.T) {
	t.Parallel()
	approval := runningApproval("a_1", "shell")
	override, err := ParseToolArgumentOverride([]byte(`{"command":"go test ./..."}`))
	if err != nil {
		t.Fatal(err)
	}
	approved := ApprovalAnswer{Decision: protocol.ApprovalApprove, ArgumentOverride: override}
	if err := ValidateAnswer(approval, approved); err != nil {
		t.Fatal(err)
	}
	cloned := CloneAnswer(approved).(ApprovalAnswer)
	if cloned.ArgumentOverride == approved.ArgumentOverride || !cloned.ArgumentOverride.Equal(approved.ArgumentOverride) {
		t.Fatal("approval answer did not deep-clone its argument override")
	}
	denied := approved
	denied.Decision = protocol.ApprovalDeny
	if err := ValidateAnswer(approval, denied); err == nil {
		t.Fatal("denied approval retained an argument override")
	}
}

func runningApproval(itemID, title string) Approval {
	return Approval{
		RunID: "run_1", ItemID: itemID, Title: title,
		Tool: &ToolCall{Kind: protocol.ToolCategoryShell, Name: "shell", Status: ToolRunning},
	}
}

func TestQuestionFieldEnforcesRuntimePresentationBounds(t *testing.T) {
	if err := (QuestionField{Prompt: "Choose", Header: "1234567890123", Kind: QuestionText}).Validate(); err == nil {
		t.Fatal("overlong question header was accepted")
	}
	if err := (QuestionField{Prompt: "Choose", Kind: QuestionSingle, Options: []protocol.QuestionOption{{Label: "only"}}}).Validate(); err == nil {
		t.Fatal("single-option choice was accepted")
	}
	if err := (QuestionField{Prompt: "Choose", Kind: QuestionSingle, Options: []protocol.QuestionOption{
		{Label: "one"}, {Label: "two"}, {Label: "three"}, {Label: "four"}, {Label: "five"},
	}}).Validate(); err == nil {
		t.Fatal("oversized option set was accepted")
	}
}

func TestQuestionEnforcesRuntimeFieldCount(t *testing.T) {
	fields := make([]QuestionField, 5)
	for index := range fields {
		fields[index] = QuestionField{Prompt: "Prompt", Kind: QuestionText}
	}
	question := Question{RunID: "run_1", ItemID: "q_1", Title: "Configuration", Fields: fields}
	if err := question.Validate(); err == nil {
		t.Fatal("question with too many fields was accepted")
	}
}

func TestCompletedQuestionOwnsAndValidatesAcceptedAnswers(t *testing.T) {
	t.Parallel()

	question := Question{
		RunID: "run_1", ItemID: "q_1", Title: "Configuration",
		Fields: []QuestionField{
			{Prompt: "Target", Kind: QuestionText},
			{Prompt: "Checks", Kind: QuestionMulti, Options: []protocol.QuestionOption{{Label: "unit"}, {Label: "smoke"}}},
		},
		Answers: [][]string{{"linux"}, {"unit", "smoke"}},
	}
	if err := question.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := validateInterrupt(question); err == nil {
		t.Fatal("answered transcript question was accepted as a pending interrupt")
	}
	cloned := question.Clone()
	if !question.Equal(cloned) {
		t.Fatal("question clone changed its value")
	}
	pending := question.Clone()
	pending.Answers = nil
	if question.Equal(pending) {
		t.Fatal("completed question equals its pending form")
	}
	cloned.Answers[0][0] = "mutated"
	if question.Answers[0][0] == "mutated" || question.Equal(cloned) {
		t.Fatal("question clone shares accepted answer storage")
	}

	invalid := question.Clone()
	invalid.Answers[1] = []string{"integration"}
	if err := invalid.Validate(); err == nil {
		t.Fatal("question accepted an answer outside its declared options")
	}
}
