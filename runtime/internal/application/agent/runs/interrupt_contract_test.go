package runs

import (
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestResolveResumeResponsesValidatesExactTypedCoverage(t *testing.T) {
	approvalPending := resolutionFixture{interrupts: ([]transcript.Interrupt{{
		ItemID: "item_approval",
		Kind:   run.InterruptApproval,
		Approval: &transcript.Approval{
			Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium", Rememberable: true,
		},
	}}), bindings: []InterruptBinding{{
		InterruptItemID: "item_approval", MemberID: "member_approval", RequestID: "request_approval", ToolCallID: "call_approval",
	}}}
	answers, err := resolveResumeResponses(approvalPending.interrupts, approvalPending.bindings, []ResumeResponse{{
		ItemID: "item_approval",
		Kind:   run.InterruptApproval,
		Approval: &ApprovalResponse{
			Approved: true, Arguments: `{"command":"echo edited","description":"Print edited"}`, RememberScope: approval.ScopeSession,
		},
	}})
	if err != nil {
		t.Fatalf("approval: %v", err)
	}
	if len(answers) != 1 {
		t.Fatalf("approval answers = %#v", answers)
	}
	resolution := answers[0].Resolution
	if resolution.Approval().Decision != approval.Allow || resolution.Approval().EditedArguments != `{"command":"echo edited","description":"Print edited"}` || resolution.Approval().RememberScope != approval.ScopeSession {
		t.Fatalf("approval resolution = %#v", resolution)
	}
	deniedAnswers, err := resolveResumeResponses(approvalPending.interrupts, approvalPending.bindings, []ResumeResponse{{
		ItemID: "item_approval",
		Kind:   run.InterruptApproval,
		Approval: &ApprovalResponse{
			Approved: false, Reason: "unsafe command",
		},
	}})
	if err != nil || len(deniedAnswers) != 1 {
		t.Fatalf("denial answers = %#v, %v", deniedAnswers, err)
	}
	denied := deniedAnswers[0].Resolution
	if (denied.Approval().Decision == approval.Allow) || denied.Approval().Reason != "unsafe command" {
		t.Fatalf("denial resolution = %#v", denied)
	}

	questionPending := resolutionFixture{interrupts: ([]transcript.Interrupt{{
		ItemID: "item_question",
		Kind:   run.InterruptQuestion,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{
			Prompt: "Choose", Kind: transcript.QuestionChoice,
			Options: []transcript.QuestionOption{{Label: "Go"}, {Label: "Stop"}},
		}}},
	}}), bindings: []InterruptBinding{{
		InterruptItemID: "item_question", MemberID: "member_question", RequestID: "request_question",
	}}}
	answers, err = resolveResumeResponses(questionPending.interrupts, questionPending.bindings, []ResumeResponse{{
		ItemID: "item_question",
		Kind:   run.InterruptQuestion,
		Question: &QuestionResponse{
			Answers: [][]string{{"Go"}},
		},
	}})
	if err != nil {
		t.Fatalf("question: %v", err)
	}
	resolution = answers[0].Resolution
	if resolution.Kind() != run.InterruptQuestion || len(resolution.Question().Answers()) != 1 || len(resolution.Question().Answers()[0]) != 1 || resolution.Question().Answers()[0][0] != "Go" {
		t.Fatalf("question resolution = %#v", resolution)
	}

	tests := []struct {
		name      string
		pending   resolutionFixture
		responses []ResumeResponse
		want      error
	}{
		{name: "missing", pending: approvalPending, want: ErrInvalidInterruptResponse},
		{name: "unknown item", pending: approvalPending, responses: []ResumeResponse{{
			ItemID: "ghost", Kind: run.InterruptApproval, Approval: &ApprovalResponse{Approved: true},
		}}, want: ErrInterruptNotOpen},
		{name: "wrong kind", pending: approvalPending, responses: []ResumeResponse{{
			ItemID: "item_approval", Kind: run.InterruptQuestion, Question: &QuestionResponse{},
		}}, want: ErrInvalidInterruptResponse},
		{name: "duplicate", pending: approvalPending, responses: []ResumeResponse{
			{ItemID: "item_approval", Kind: run.InterruptApproval, Approval: &ApprovalResponse{Approved: true}},
			{ItemID: "item_approval", Kind: run.InterruptApproval, Approval: &ApprovalResponse{Approved: true}},
		}, want: ErrInvalidInterruptResponse},
		{name: "invalid choice", pending: questionPending, responses: []ResumeResponse{{
			ItemID: "item_question", Kind: run.InterruptQuestion,
			Question: &QuestionResponse{Answers: [][]string{{"Rust"}}},
		}}, want: ErrInvalidInterruptResponse},
		{name: "one-off approval cannot be remembered", pending: resolutionFixture{
			interrupts: ([]transcript.Interrupt{{
				ItemID: "item_one_off", Kind: run.InterruptApproval,
				Approval: &transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium"},
			}}),
			bindings: []InterruptBinding{{
				InterruptItemID: "item_one_off", MemberID: "member_one_off", RequestID: "request_one_off", ToolCallID: "call_one_off",
			}}}, responses: []ResumeResponse{{
			ItemID: "item_one_off", Kind: run.InterruptApproval,
			Approval: &ApprovalResponse{Approved: true, RememberScope: approval.ScopeSession},
		}}, want: ErrInvalidInterruptResponse},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveResumeResponses(test.pending.interrupts, test.pending.bindings, test.responses)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestResolveResumeResponsesPreservesCompleteBarrierInCanonicalOrder(t *testing.T) {
	pending := resolutionFixture{
		interrupts: ([]transcript.Interrupt{
			{
				ItemID: "item_a",
				Kind:   run.InterruptApproval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium",
				},
			},
			{
				ItemID: "item_b",
				Kind:   run.InterruptApproval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "write"}, Risk: "medium",
				},
			},
		}),
		bindings: []InterruptBinding{
			{InterruptItemID: "item_a", MemberID: "member_a", RequestID: "request_a", ToolCallID: "call_a"},
			{InterruptItemID: "item_b", MemberID: "member_b", RequestID: "request_b", ToolCallID: "call_b"},
		}}
	answers, err := resolveResumeResponses(pending.interrupts, pending.bindings, []ResumeResponse{
		{
			ItemID:   "item_b",
			Kind:     run.InterruptApproval,
			Approval: &ApprovalResponse{Approved: false, Reason: "skip b"},
		},
		{
			ItemID:   "item_a",
			Kind:     run.InterruptApproval,
			Approval: &ApprovalResponse{Approved: true},
		},
	})
	if err != nil {
		t.Fatalf("resolve complete barrier: %v", err)
	}
	if len(answers) != 2 {
		t.Fatalf("answers = %#v", answers)
	}
	if answers[0].InterruptItemID != "item_a" ||
		answers[0].MemberID != "member_a" ||
		answers[0].RequestID != "request_a" ||
		answers[0].Resolution.Approval().Decision != approval.Allow {
		t.Fatalf("answer[0] = %#v", answers[0])
	}
	if answers[1].InterruptItemID != "item_b" ||
		answers[1].MemberID != "member_b" ||
		answers[1].RequestID != "request_b" ||
		(answers[1].Resolution.Approval().Decision == approval.Allow) ||
		answers[1].Resolution.Approval().Reason != "skip b" {
		t.Fatalf("answer[1] = %#v", answers[1])
	}
}

func TestResolveQuestionResponseUsesOrderedExactAnswers(t *testing.T) {
	question := &transcript.Question{Fields: []transcript.QuestionField{
		{Prompt: "Name it", Kind: transcript.QuestionText},
		{
			Prompt: "Choose", Kind: transcript.QuestionChoice, Multiple: true, AllowCustom: true,
			Options: []transcript.QuestionOption{{Label: "A"}, {Label: "B"}},
		},
	}}
	pending := transcript.Interrupt{ItemID: "item_question", Kind: run.InterruptQuestion, Question: question}
	response := func(answers [][]string) ResumeResponse {
		return ResumeResponse{
			ItemID: "item_question", Kind: run.InterruptQuestion,
			Question: &QuestionResponse{Answers: answers},
		}
	}

	resolution, err := resolveQuestionResponse(pending, response([][]string{{"name"}, {"A", "custom"}}))
	if err != nil {
		t.Fatalf("valid ordered answer: %v", err)
	}
	if resolution.Kind() != run.InterruptQuestion || !slices.Equal(resolution.Question().Answers()[0], []string{"name"}) ||
		!slices.Equal(resolution.Question().Answers()[1], []string{"A", "custom"}) {
		t.Fatalf("resolution = %#v", resolution)
	}

	skipped, err := resolveQuestionResponse(pending, response([][]string{{}, {}}))
	if err != nil {
		t.Fatalf("explicit skipped answers: %v", err)
	}
	if skipped.Kind() != run.InterruptQuestion || len(skipped.Question().Answers()) != len(question.Fields) ||
		len(skipped.Question().Answers()[0]) != 0 || len(skipped.Question().Answers()[1]) != 0 {
		t.Fatalf("skipped resolution = %#v", skipped)
	}

	tests := []struct {
		name      string
		answers   [][]string
		wantIndex int
	}{
		{name: "wrong field count", answers: [][]string{{"name"}}, wantIndex: -1},
		{name: "empty text", answers: [][]string{{""}, {"A"}}, wantIndex: 0},
		{name: "two custom choices", answers: [][]string{{"name"}, {"custom-1", "custom-2"}}, wantIndex: 1},
		{name: "duplicate choices", answers: [][]string{{"name"}, {"A", "A"}}, wantIndex: 1},
		{name: "padded choice", answers: [][]string{{"name"}, {" A "}}, wantIndex: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveQuestionResponse(pending, response(test.answers))
			var answerError *QuestionAnswerError
			if !errors.As(err, &answerError) || answerError.Index != test.wantIndex ||
				!errors.Is(err, ErrInvalidInterruptResponse) {
				t.Fatalf("error = %v, answer error = %#v", err, answerError)
			}
		})
	}

	question.Fields[1].AllowCustom = false
	if _, err := resolveQuestionResponse(pending, response([][]string{{"name"}, {"custom"}})); err == nil {
		t.Fatal("closed choice accepted custom value")
	}
}

func TestInterruptAnswerRefusesAnAnswerForAnotherKind(t *testing.T) {
	for _, request := range []transcript.Interrupt{
		{Kind: run.InterruptApproval, Approval: &transcript.Approval{}},
		{Kind: run.InterruptQuestion, Question: &transcript.Question{}},
	} {
		answer := testsupport.QuestionAnswer(t, [][]string{{"name"}})
		if request.Kind == run.InterruptQuestion {
			answer = testsupport.ApprovalAnswer(t, approval.Resolution{Decision: approval.Allow})
		}
		if err := (InterruptAnswer{Resolution: answer}).validateResolution(request); err == nil {
			t.Fatal("cross-kind answer accepted")
		}
	}
}

// resolutionFixture is an open interrupt set as resolveResumeResponses reads
// it: interrupts projected from their Items, and their bindings.
type resolutionFixture struct {
	interrupts []transcript.Interrupt
	bindings   []InterruptBinding
}
