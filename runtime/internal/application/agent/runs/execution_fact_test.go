package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

func TestExecutionFactReceiptPreservesTheProducerCancellationCause(t *testing.T) {
	_, receipt, err := NewExecutionFactCommit(SteerMessagesApplied{})
	if err != nil {
		t.Fatalf("new execution fact commit: %v", err)
	}
	want := errors.New("interaction producer retired")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(want)
	if err := receipt.Await(ctx); !errors.Is(err, want) {
		t.Fatalf("receipt error = %v, want producer cause", err)
	}
}

func TestNewExecutionFactCommitRejectsUnsupportedFactRepresentation(t *testing.T) {
	if _, _, err := NewExecutionFactCommit(&ModelCallStarted{}); err == nil {
		t.Fatal("execution fact commit accepted a pointer representation the reducer cannot consume")
	}
}

func TestExecutionFactCommitOwnsMutableFacts(t *testing.T) {
	model := ModelCallCompleted{
		Message: corechat.NewAssistantMessage(corechat.NewTextPart("original")),
		ByModel: []accounting.ModelUsage{{Model: "model-original", Calls: 1}},
	}
	modelCommit, _, err := NewExecutionFactCommit(model)
	if err != nil {
		t.Fatalf("new model fact commit: %v", err)
	}
	model.Message.Parts[0].Text = "changed"
	model.ByModel[0].Model = "model-changed"
	projectedModel := modelCommit.Fact().(ModelCallCompleted)
	projectedModel.Message.Parts[0].Text = "projected"
	projectedModel.ByModel[0].Model = "model-projected"
	ownedModel := modelCommit.Fact().(ModelCallCompleted)
	if ownedModel.Message.Text() != "original" || ownedModel.ByModel[0].Model != "model-original" {
		t.Fatalf("owned model fact = %+v", ownedModel)
	}

	modelResult := corechat.ToolResult{
		ID: "provider-call", Name: "tool", Output: corechat.NewTextToolOutput("original"),
	}
	failure := tool.Failure{Kind: tool.FailureExecution, Detail: "original"}
	toolFact := ToolCallFinished{
		ModelResult: &modelResult, MutatedPaths: []string{"/original"}, Failure: &failure,
	}
	toolCommit, _, err := NewExecutionFactCommit(toolFact)
	if err != nil {
		t.Fatalf("new Tool fact commit: %v", err)
	}
	modelResult.Name = "changed"
	toolFact.MutatedPaths[0] = "/changed"
	failure.Detail = "changed"
	projectedTool := toolCommit.Fact().(ToolCallFinished)
	projectedTool.ModelResult.Name = "projected"
	projectedTool.MutatedPaths[0] = "/projected"
	projectedTool.Failure.Detail = "projected"
	ownedTool := toolCommit.Fact().(ToolCallFinished)
	if ownedTool.ModelResult.Name != "tool" || ownedTool.MutatedPaths[0] != "/original" ||
		ownedTool.Failure.Detail != "original" {
		t.Fatalf("owned Tool fact = %+v", ownedTool)
	}

	question := QuestionPrompt{Fields: []QuestionFieldSpec{{
		Prompt: "choose", Options: []QuestionOptionSpec{{Label: "original"}},
	}}}
	interrupted := SegmentInterrupted{Interrupts: []Interrupt{{
		Kind: interrupt.Question, Question: &question,
	}}}
	interruptCommit, _, err := NewExecutionFactCommit(interrupted)
	if err != nil {
		t.Fatalf("new interrupt fact commit: %v", err)
	}
	question.Fields[0].Options[0].Label = "changed"
	projectedInterrupt := interruptCommit.Fact().(SegmentInterrupted)
	projectedInterrupt.Interrupts[0].Question.Fields[0].Options[0].Label = "projected"
	ownedInterrupt := interruptCommit.Fact().(SegmentInterrupted)
	if ownedInterrupt.Interrupts[0].Question.Fields[0].Options[0].Label != "original" {
		t.Fatalf("owned interrupt fact = %+v", ownedInterrupt)
	}

	usage := SegmentUsage{ByModel: []accounting.ModelUsage{{Model: "model-original", Calls: 1}}}
	runFailure := run.Failure{Kind: run.FailureInternal, Detail: "original"}
	ended := SegmentEnded{Failure: &runFailure, Usage: &usage}
	endedCommit, _, err := NewExecutionFactCommit(ended)
	if err != nil {
		t.Fatalf("new segment-end fact commit: %v", err)
	}
	usage.ByModel[0].Model = "model-changed"
	runFailure.Detail = "changed"
	projectedEnd := endedCommit.Fact().(SegmentEnded)
	projectedEnd.Usage.ByModel[0].Model = "model-projected"
	projectedEnd.Failure.Detail = "projected"
	ownedEnd := endedCommit.Fact().(SegmentEnded)
	if ownedEnd.Usage.ByModel[0].Model != "model-original" || ownedEnd.Failure.Detail != "original" {
		t.Fatalf("owned segment-end fact = %+v", ownedEnd)
	}

	steer := SteerMessagesApplied{Messages: []AppliedSteerMessage{{
		Content: []transcript.ContentBlock{{Kind: transcript.ImageContent, Bytes: []byte("original")}},
	}}}
	steerCommit, _, err := NewExecutionFactCommit(steer)
	if err != nil {
		t.Fatalf("new steer fact commit: %v", err)
	}
	steer.Messages[0].Content[0].Bytes[0] = 'x'
	projectedSteer := steerCommit.Fact().(SteerMessagesApplied)
	projectedSteer.Messages[0].Content[0].Bytes[0] = 'y'
	ownedSteer := steerCommit.Fact().(SteerMessagesApplied)
	if string(ownedSteer.Messages[0].Content[0].Bytes) != "original" {
		t.Fatalf("owned steer fact = %+v", ownedSteer)
	}
}

func TestExecutorMemberValidate(t *testing.T) {
	tests := []struct {
		name    string
		member  ExecutorMember
		wantErr string
	}{
		{
			name: "root executor member",
			member: ExecutorMember{
				MemberID: "member_root",
			},
		},
		{
			name: "child executor member",
			member: ExecutorMember{
				MemberID:    "member_child",
				ParentID:    "member_root",
				SpawnCallID: "call_delegate",
			},
		},
		{
			name: "failure before executor member creation",
		},
		{
			name:    "executor member whitespace",
			member:  ExecutorMember{MemberID: " member_root"},
			wantErr: "runs: executor member identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "parent whitespace",
			member: ExecutorMember{
				MemberID: "member_child",
				ParentID: "member_root ",
			},
			wantErr: "runs: executor parent: executor member identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "spawn call whitespace",
			member: ExecutorMember{
				MemberID:    "member_child",
				ParentID:    "member_root",
				SpawnCallID: " call_delegate",
			},
			wantErr: "runs: executor spawn call: executor effect identity must contain 1 to 256 URI-safe ASCII bytes",
		},
		{
			name: "empty executor member with parent",
			member: ExecutorMember{
				ParentID: "member_root",
			},
			wantErr: "runs: empty executor member id cannot carry parent or spawn-call identity",
		},
		{
			name: "empty executor member with spawn call",
			member: ExecutorMember{
				SpawnCallID: "call_delegate",
			},
			wantErr: "runs: empty executor member id cannot carry parent or spawn-call identity",
		},
		{
			name: "self-parent",
			member: ExecutorMember{
				MemberID: "member_1",
				ParentID: "member_1",
			},
			wantErr: "runs: executor member cannot parent itself",
		},
		{
			name: "root with spawn call",
			member: ExecutorMember{
				MemberID:    "member_root",
				SpawnCallID: "call_delegate",
			},
			wantErr: "runs: root executor member cannot carry spawn-call identity",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.member.Validate()
			switch {
			case test.wantErr == "" && err != nil:
				t.Fatalf("Validate() error = %v, want nil", err)
			case test.wantErr != "" && err == nil:
				t.Fatalf("Validate() error = nil, want %q", test.wantErr)
			case test.wantErr != "" && err.Error() != test.wantErr:
				t.Fatalf("Validate() error = %q, want %q", err, test.wantErr)
			}
		})
	}
}

func TestExecutorEventValidateRequiresPayload(t *testing.T) {
	err := (ExecutorEvent{Member: ExecutorMember{MemberID: "member_root"}}).Validate()
	if err == nil || err.Error() != "runs: executor event payload is required" {
		t.Fatalf("Validate() error = %v, want missing-payload error", err)
	}
}
