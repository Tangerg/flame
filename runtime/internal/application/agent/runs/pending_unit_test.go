package runs

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestResumeClaimSettlesReviewedToolCallsFromTheirItems(t *testing.T) {
	pending := validTreePending()
	answers := make([]InterruptAnswer, len(pending.Bindings))
	for index, binding := range pending.Bindings {
		answers[index] = InterruptAnswer{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			Resolution:      testsupport.ApprovalAnswer(t, approval.Resolution{Decision: approval.DecisionOf(index == 0)}),
		}
	}
	items := fixtureItems(pending)
	written := items["item_b"]
	items["item_b"] = testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: written.ID(), SessionID: written.SessionID(), RunID: written.RunID(),
		Kind: transcript.ToolCall, Status: transcript.ItemRunning, OccurredAt: written.OccurredAt(),
		Tool: &transcript.ToolInvocation{Name: "write"},
	})
	claim, err := NewResumeAcceptance(
		fixtureSessionID, pending, items, answers,
	)
	if err != nil {
		t.Fatalf("NewResumeAcceptance: %v", err)
	}
	replacements, err := claim.ItemReplacements()
	if err != nil {
		t.Fatalf("ItemReplacements: %v", err)
	}
	if len(replacements) != 2 ||
		replacements[0].Expected().ID() != "item_grandchild" ||
		replacements[0].State().ApprovalDecision() != approval.Allow ||
		replacements[1].Expected().ID() != "item_b" ||
		replacements[1].State().ApprovalDecision() != approval.Deny {
		t.Fatalf("Tool approval replacements = %+v", replacements)
	}
	calls := claim.approvalCalls()
	if calls["item_grandchild"] != "call_grandchild" || calls["item_b"] != "call_b" {
		t.Fatalf("approval call bindings = %+v", calls)
	}

	claim.answers[0].Resolution = testsupport.QuestionAnswer(t, [][]string{{"unexpected"}})
	if err := claim.Validate(); err == nil || !strings.Contains(err.Error(), "answer kind differs") {
		t.Fatalf("Validate cross-kind resolution error = %v", err)
	}
}

func TestResumeClaimOwnsPendingAndQuestionAnswers(t *testing.T) {
	pending := validTreePending()
	pending.Interrupts = []OpenInterrupt{{ItemID: "item_grandchild"}}
	items := map[string]transcript.Item{"item_grandchild": testsupport.MustRestoreItem(testsupport.ItemInput{
		ID: "item_grandchild", SessionID: fixtureSessionID, RunID: "run_grandchild",
		Kind: transcript.QuestionItem, OccurredAt: pending.CreatedAt,
		Question: &transcript.Question{Fields: []transcript.QuestionField{{
			Prompt: "Continue?", Kind: questioncontract.Choice,
			Options: []transcript.QuestionOption{{Label: "yes"}, {Label: "no"}},
		}}},
	})}
	pending.Bindings = []InterruptBinding{{
		InterruptItemID: "item_grandchild", MemberID: "member_grandchild", RequestID: "request_grandchild",
	}}
	answers := []InterruptAnswer{{
		InterruptItemID: "item_grandchild", MemberID: "member_grandchild", RequestID: "request_grandchild",
		Resolution: testsupport.QuestionAnswer(t, [][]string{{"yes"}}),
	}}

	claim, err := NewResumeAcceptance(
		fixtureSessionID, pending, items, answers,
	)
	if err != nil {
		t.Fatalf("NewResumeAcceptance: %v", err)
	}
	delete(items, "item_grandchild")
	pending.Continuations[0].MemberID = "member_changed"
	answers[0].Resolution.Question().Answers()[0][0] = "changed input"

	ownedPending := claim.Pending()
	ownedAnswers := claim.Answers()
	ownedPending.Continuations[0].MemberID = "member_changed_again"
	ownedAnswers[0].Resolution.Question().Answers()[0][0] = "changed accessor"

	gotPending := claim.Pending()
	gotAnswers := claim.Answers()
	if gotPending.Continuations[0].MemberID != "member_grandchild" ||
		gotAnswers[0].Resolution.Question().Answers()[0][0] != "yes" {
		t.Fatalf("Resume claim ownership = pending:%+v answers:%+v", gotPending, gotAnswers)
	}
	if err := claim.Validate(); err != nil {
		t.Fatalf("Validate after caller mutations: %v", err)
	}
}

func TestPendingValidateRequiresOneCanonicalConnectedTree(t *testing.T) {
	pending := validTreePending()
	if err := pending.Validate(); err != nil {
		t.Fatalf("Validate canonical tree: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Pending)
		want   string
	}{
		{
			name: "duplicate opaque executor member binding",
			mutate: func(p *Pending) {
				p.Continuations[0].MemberID = p.Continuations[1].MemberID
			},
			want: "duplicate continuation member",
		},
		{
			name: "binding order differs from interrupt order",
			mutate: func(p *Pending) {
				p.Bindings[0], p.Bindings[1] = p.Bindings[1], p.Bindings[0]
			},
			want: "canonical interrupt order",
		},
		{
			name: "pending identity is not canonical",
			mutate: func(p *Pending) {
				p.ExecutorID = " turn_1"
			},
			want: "executor identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "pending creation time is not UTC",
			mutate: func(p *Pending) {
				p.CreatedAt = p.CreatedAt.In(time.FixedZone("offset", 60))
			},
			want: "pending creation time is required in UTC",
		},
		{
			name: "continuation identity is not canonical",
			mutate: func(p *Pending) {
				p.Continuations[0].MemberID += " "
			},
			want: "executor member identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "input request identity is not canonical",
			mutate: func(p *Pending) {
				p.Bindings[0].RequestID += " "
			},
			want: "executor request identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "approval Tool call identity is missing",
			mutate: func(p *Pending) {
				p.Bindings[0].ToolCallID = ""
			},
			want: "invocation identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "approval Tool call identity is not canonical",
			mutate: func(p *Pending) {
				p.Bindings[0].ToolCallID += " "
			},
			want: "invocation identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "approval Tool call is also drained",
			mutate: func(p *Pending) {
				p.Continuations[0].DrainedTools = []DrainedTool{{
					ItemID: "item_drained",
					CallID: "call_grandchild",
				}}
			},
			want: "approval Tool call \"call_grandchild\" is also drained",
		},
		{
			name: "interrupt identity is not canonical",
			mutate: func(p *Pending) {
				p.Interrupts[0].ItemID += " "
			},
			want: "item identity contains whitespace or a non-printing character",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := validTreePending()
			test.mutate(&candidate)
			err := candidate.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate error = %v, want containing %q", err, test.want)
			}
		})
	}
}

// TestPendingRunTreeOwnsTopologyAndContract proves the checks a Pending cannot
// make alone: its continuation order is its Runs' canonical postorder, and the
// root Run admits every interrupt kind it parks on.
func TestPendingRunTreeOwnsTopologyAndContract(t *testing.T) {
	pending := validTreePending()
	facts := fixtureFacts(pending)
	if err := validatePendingRunTree(pending, parkedTree(pending, facts)); err != nil {
		t.Fatalf("validatePendingRunTree canonical tree: %v", err)
	}

	reordered := validTreePending()
	reordered.Continuations[0], reordered.Continuations[1] = reordered.Continuations[1], reordered.Continuations[0]
	if err := validatePendingRunTree(reordered, parkedTree(reordered, facts)); err == nil || !strings.Contains(err.Error(), "canonical postorder") {
		t.Fatalf("non canonical order error = %v", err)
	}

	unadmitted := facts
	unadmitted.capabilities = run.Capabilities{ChildRuns: true, InterruptKinds: []run.InterruptKind{run.InterruptQuestion}}
	if err := validatePendingRunTree(pending, parkedTree(pending, unadmitted)); err == nil || !strings.Contains(err.Error(), "outside the root Run's capabilities") {
		t.Fatalf("unadmitted interrupt kind error = %v", err)
	}

	solitary := facts
	solitary.capabilities.ChildRuns = false
	if err := validatePendingRunTree(pending, parkedTree(pending, solitary)); err == nil || !strings.Contains(err.Error(), "child-Run capability") {
		t.Fatalf("child Runs without capability error = %v", err)
	}
}

func TestPendingValidatesExactReadIdentities(t *testing.T) {
	pending := validTreePending()
	if err := pending.ValidateForRoot(pending.RootRunID); err != nil {
		t.Fatalf("ValidateForRoot exact Pending: %v", err)
	}
	if err := pending.ValidateForRoot("run_other"); err == nil || !strings.Contains(err.Error(), "requested identity") {
		t.Fatalf("ValidateForRoot mismatched Pending error = %v", err)
	}
	if err := (Pending{}).ValidateForRoot("run_root"); err == nil {
		t.Fatal("ValidateForRoot accepted invalid Pending")
	}
}

func TestPendingEqualUsesLogicalDurableValue(t *testing.T) {
	left := validTreePending()
	right := left
	right.CreatedAt = right.CreatedAt.In(time.FixedZone("equal-instant", 8*60*60))
	right.Continuations = slices.Clone(right.Continuations)
	right.Continuations[0].DrainedTools = []DrainedTool{}
	if !left.Equal(right) {
		t.Fatal("Equal rejected equivalent time and empty collection representations")
	}
	right.ExecutorID = "turn_2"
	if left.Equal(right) {
		t.Fatal("Equal accepted a different executor identity")
	}
}

func TestExecutionMemberRequiresExactModelSelection(t *testing.T) {
	member := ExecutionMember{RunID: "run_1", MemberID: "member_1"}
	if err := member.Validate(); err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("Validate without waiting member model selection error = %v", err)
	}
}

func validTreePending() Pending {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	return Pending{
		RootRunID:  "run_root",
		ExecutorID: "turn_1",
		Interrupts: OpenInterruptsOf([]transcript.Interrupt{
			{
				ItemID: "item_grandchild", ItemOccurredAt: createdAt,
				RunID: "run_grandchild",
				Kind:  run.InterruptApproval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium",
				},
			},
			{
				ItemID: "item_b", ItemOccurredAt: createdAt,
				RunID: "run_b",
				Kind:  run.InterruptApproval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "write"}, Risk: "medium",
				},
			},
		}),
		Bindings: []InterruptBinding{
			{InterruptItemID: "item_grandchild", MemberID: "member_grandchild", RequestID: "request_grandchild", ToolCallID: "call_grandchild"},
			{InterruptItemID: "item_b", MemberID: "member_b", RequestID: "request_b", ToolCallID: "call_b"},
		},
		Continuations: []Continuation{
			{
				RunID:    "run_grandchild",
				MemberID: "member_grandchild",
			},
			{
				RunID:    "run_a",
				MemberID: "member_a",
			},
			{
				RunID:    "run_b",
				MemberID: "member_b",
			},
			{
				RunID:    "run_root",
				MemberID: "member_root",
			},
		},
		CreatedAt: createdAt}
}

func TestAcceptedAnswersHaveOneFreshAndRecoveryProjection(t *testing.T) {
	for _, shape := range []string{"question", "approval", "mixed"} {
		t.Run(shape, func(t *testing.T) {
			pending := validTreePending()
			for index := range pending.Interrupts {
				if shape == "question" || (shape == "mixed" && index == 0) {
					pending.Interrupts[index].Approval = nil
					binding := pending.Bindings[index]
					pending.Bindings[index] = InterruptBinding{InterruptItemID: binding.InterruptItemID, MemberID: binding.MemberID, RequestID: binding.RequestID}
				}
			}
			items := fixtureItems(pending)
			open, err := pending.ProjectInterrupts(items)
			if err != nil {
				t.Fatal(err)
			}
			responses := make([]ResumeResponse, len(open))
			for index, request := range open {
				responses[index] = ResumeResponse{ItemID: request.ItemID, Kind: request.Kind}
				if request.Kind == run.InterruptQuestion {
					responses[index].Question = &QuestionResponse{Answers: [][]string{{"chosen"}}}
				} else {
					responses[index].Approval = &ApprovalResponse{Approved: index == 0}
				}
			}
			resolved, err := resolveResumeResponses(open, pending.Bindings, responses)
			if err != nil {
				t.Fatal(err)
			}
			acceptance, err := NewResumeAcceptance(fixtureSessionID, pending, items, resolved)
			if err != nil {
				t.Fatal(err)
			}
			replacements, err := acceptance.ItemReplacements()
			if err != nil {
				t.Fatal(err)
			}
			for _, replacement := range replacements {
				stored, err := transcript.RestoreItem(replacement.State().Snapshot())
				if err != nil {
					t.Fatal(err)
				}
				items[stored.ID()] = stored
			}
			revived, err := (AcceptedContinuation{SessionID: fixtureSessionID, Pending: pending}).answers(items)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(acceptance.Answers(), revived) {
				t.Fatalf("accepted answers changed after restore: fresh=%+v revived=%+v", acceptance.Answers(), revived)
			}
		})
	}
}
