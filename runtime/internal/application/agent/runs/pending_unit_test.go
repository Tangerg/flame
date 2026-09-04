package runs

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestResumeClaimDerivesExactToolApprovalResolutions(t *testing.T) {
	pending := validTreePending()
	answers := make([]InterruptAnswer, len(pending.Bindings))
	for index, binding := range pending.Bindings {
		answers[index] = InterruptAnswer{
			InterruptItemID: binding.InterruptItemID,
			MemberID:        binding.MemberID,
			RequestID:       binding.RequestID,
			Resolution:      interrupt.Resolution{Approved: index == 0},
		}
	}
	claim := ResumeClaimCommit{
		CommitID: testCommitID("run_commit_approval"), Expected: pending, Answers: answers,
		ClaimedAt: pending.CreatedAt.Add(time.Second),
	}
	if err := claim.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	resolutions, err := claim.ToolApprovalResolutions()
	if err != nil {
		t.Fatalf("ToolApprovalResolutions: %v", err)
	}
	if len(resolutions) != 2 ||
		resolutions[0].Identity.ItemID != "item_grandchild" ||
		resolutions[0].CallID != "call_grandchild" ||
		resolutions[0].Invocation.Name != "shell" ||
		resolutions[0].Decision != approval.Allow ||
		resolutions[1].Identity.ItemID != "item_b" ||
		resolutions[1].CallID != "call_b" ||
		resolutions[1].Invocation.Name != "write" ||
		resolutions[1].Decision != approval.Deny {
		t.Fatalf("Tool approval resolutions = %+v", resolutions)
	}

	claim.Answers[0].Resolution.Answers = [][]string{{"unexpected"}}
	if err := claim.Validate(); err == nil || !strings.Contains(err.Error(), "cannot carry question answers") {
		t.Fatalf("Validate cross-kind resolution error = %v", err)
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
			name: "non canonical continuation order",
			mutate: func(p *Pending) {
				p.Continuations[0], p.Continuations[1] = p.Continuations[1], p.Continuations[0]
			},
			want: "canonical postorder",
		},
		{
			name: "duplicate opaque executor member binding",
			mutate: func(p *Pending) {
				p.Continuations[0].MemberID = p.Continuations[1].MemberID
			},
			want: "duplicate continuation member",
		},
		{
			name: "disconnected Run",
			mutate: func(p *Pending) {
				p.Continuations[1].Lineage.ParentRunID = "run_b"
				p.Continuations[2].Lineage.ParentRunID = "run_a"
			},
			want: "cycle",
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
			want: "executor effect identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "approval Tool call identity is not canonical",
			mutate: func(p *Pending) {
				p.Bindings[0].ToolCallID += " "
			},
			want: "executor effect identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "approval Tool call is also drained",
			mutate: func(p *Pending) {
				p.Continuations[0].DrainedTools = []DrainedTool{{
					ItemID: "item_drained", ItemOccurredAt: p.CreatedAt,
					CallID: "call_grandchild", Name: "shell", Arguments: `{}`,
				}}
			},
			want: "approval Tool call \"call_grandchild\" is also drained",
		},
		{
			name: "approval Tool call is already committed",
			mutate: func(p *Pending) {
				p.Continuations[0].CommittedTools = []CommittedTool{{
					ItemID: "item_committed", CallID: "call_grandchild",
					Name: "shell", Arguments: `{}`, Failure: tool.Failure{Kind: tool.FailureInternal},
				}}
			},
			want: "approval Tool call \"call_grandchild\" is already committed",
		},
		{
			name: "interrupt identity is not canonical",
			mutate: func(p *Pending) {
				p.Interrupts[0].ItemID += " "
			},
			want: "item identity contains whitespace or a non-printing character",
		},
		{
			name: "interrupt item occurrence is missing",
			mutate: func(p *Pending) {
				p.Interrupts[0].ItemOccurredAt = time.Time{}
			},
			want: "item occurrence time is required",
		},
		{
			name: "drained tool item occurrence is missing",
			mutate: func(p *Pending) {
				p.Continuations[0].DrainedTools = []DrainedTool{{
					ItemID: "item_open", CallID: "call_open", Name: "shell", Arguments: "{}",
				}}
			},
			want: "item occurrence time is required",
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
	if err := pending.ValidateForSession(pending.SessionID); err != nil {
		t.Fatalf("ValidateForSession exact Pending: %v", err)
	}
	if err := pending.ValidateForSession("session_other"); err == nil || !strings.Contains(err.Error(), "requested identity") {
		t.Fatalf("ValidateForSession mismatched Pending error = %v", err)
	}
}

func TestPendingEqualUsesLogicalDurableValue(t *testing.T) {
	left := validTreePending()
	right := left
	right.CreatedAt = right.CreatedAt.In(time.FixedZone("equal-instant", 8*60*60))
	right.Continuations = slices.Clone(right.Continuations)
	right.Continuations[0].DrainedTools = []DrainedTool{}
	right.Continuations[0].CommittedTools = []CommittedTool{}
	if !left.Equal(right) {
		t.Fatal("Equal rejected equivalent time and empty collection representations")
	}
	right.ExecutorID = "turn_2"
	if left.Equal(right) {
		t.Fatal("Equal accepted a different executor identity")
	}
}

func TestContinuationRequiresExactModelSelection(t *testing.T) {
	pending := validTreePending()
	pending.Continuations[0].ModelSelection = modelref.Selection{}
	if err := pending.Validate(); err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("Validate without continuation model selection error = %v", err)
	}
}

func TestWaitingMemberRequiresExactModelSelection(t *testing.T) {
	member := WaitingMember{RunID: "run_1", MemberID: "member_1"}
	if err := member.Validate(); err == nil || !strings.Contains(err.Error(), "model selection is required") {
		t.Fatalf("Validate without waiting member model selection error = %v", err)
	}
}

func validTreePending() Pending {
	createdAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	return Pending{
		RootRunID:  "run_root",
		SessionID:  "session_1",
		ExecutorID: "turn_1",
		Capabilities: run.Capabilities{
			ChildRuns:      true,
			InterruptKinds: []interrupt.Kind{interrupt.Approval},
		},
		Interrupts: []transcript.Interrupt{
			{
				ItemID: "item_grandchild", ItemOccurredAt: createdAt,
				RunID: "run_grandchild",
				Kind:  interrupt.Approval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "shell"}, Risk: "medium",
				},
			},
			{
				ItemID: "item_b", ItemOccurredAt: createdAt,
				RunID: "run_b",
				Kind:  interrupt.Approval,
				Approval: &transcript.Approval{
					Tool: transcript.ToolInvocation{Name: "write"}, Risk: "medium",
				},
			},
		},
		Bindings: []InterruptBinding{
			{InterruptItemID: "item_grandchild", MemberID: "member_grandchild", RequestID: "request_grandchild", ToolCallID: "call_grandchild"},
			{InterruptItemID: "item_b", MemberID: "member_b", RequestID: "request_b", ToolCallID: "call_b"},
		},
		Continuations: []Continuation{
			{
				RunID:    "run_grandchild",
				MemberID: "member_grandchild",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_grandchild",
					ParentRunID:     "run_a",
					RootRunID:       "run_root",
				},
				ModelSelection: testsupport.DefaultModelSelection(),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:    "run_a",
				MemberID: "member_a",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_a",
					ParentRunID:     "run_root",
					RootRunID:       "run_root",
				},
				ModelSelection: testsupport.DefaultModelSelection(),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:    "run_b",
				MemberID: "member_b",
				Lineage: run.Lineage{
					SpawnedByItemID: "item_spawn_b",
					ParentRunID:     "run_root",
					RootRunID:       "run_root",
				},
				ModelSelection: testsupport.DefaultModelSelection(),
				RunCreatedAt:   createdAt,
			},
			{
				RunID:          "run_root",
				MemberID:       "member_root",
				ModelSelection: testsupport.DefaultModelSelection(),
				RunCreatedAt:   createdAt,
			},
		},
		CreatedAt: createdAt,
	}
}
