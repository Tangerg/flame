package runs

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport/runfixture"
)

func checkpointSelection(t *testing.T, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New: %v", err)
	}
	return selection
}

func TestExecutorCheckpointValidatesOnlyApplicationEnvelope(t *testing.T) {
	valid := ExecutorCheckpoint{
		RootMemberID: "root",
		Payload:      []byte(`{"executorOwned":"opaque"}`),
		BuildID:      testExecutorBuildID,
		Scope: ExecutionScope{
			SessionID:         "session-1",
			CWD:               "/workspace/project",
			Isolated:          true,
			GoalIncarnationID: "lease-1",
		},
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
		Limits: runfixture.MustLimits(run.LimitValues{
			MaxTotalTokens: runfixture.Pointer[int64](4_096),
			MaxBudgetUSD:   runfixture.Pointer(1.5),
			MaxSteps:       runfixture.Pointer(8),
		}),
		Usage: accounting.Snapshot{},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExecutorCheckpoint)
	}{
		{name: "empty root", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.RootMemberID = "" }},
		{name: "unstable root", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.RootMemberID = " root" }},
		{name: "empty payload", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.Payload = nil }},
		{name: "empty build", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.BuildID = "" }},
		{name: "unstable session", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.SessionID = " session-1" }},
		{name: "unstable cwd", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.CWD = "/workspace/project " }},
		{name: "goal incarnation whitespace", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.GoalIncarnationID = "lease 1" }},
		{name: "goal incarnation non-printing", mutate: func(checkpoint *ExecutorCheckpoint) { checkpoint.Scope.GoalIncarnationID = "lease\u200b1" }},
		{name: "goal incarnation oversized", mutate: func(checkpoint *ExecutorCheckpoint) {
			checkpoint.Scope.GoalIncarnationID = strings.Repeat("界", goalref.MaximumIncarnationCharacters+1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkpoint := valid.Clone()
			test.mutate(&checkpoint)
			if err := checkpoint.Validate(); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("Validate error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}

func TestExecutorCheckpointCloneOwnsMutableData(t *testing.T) {
	original := ExecutorCheckpoint{
		RootMemberID: "root",
		Payload:      []byte("payload"),
		BuildID:      testExecutorBuildID,
		Usage:        accounting.Snapshot{Models: []accounting.ModelUsage{{Model: "model"}}},
	}
	clone := original.Clone()
	clone.Payload[0] = 'P'
	clone.Usage.Models[0].Model = "changed"
	if string(original.Payload) != "payload" || original.Usage.Models[0].Model != "model" {
		t.Fatalf("Clone shares mutable storage with original: %+v", original)
	}
}

func TestExecutorCheckpointValidatesCrossAggregateOwnership(t *testing.T) {
	checkpoint := ExecutorCheckpoint{
		RootMemberID: "member-root",
		Payload:      []byte("opaque"),
		BuildID:      testExecutorBuildID,
		Scope: ExecutionScope{
			SessionID:    "session-1",
			CWD:          "/scratch/project",
			WorkspaceCWD: "/workspace/project",
		},
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
	}
	expected := ExecutorCheckpointExpectation{
		RootMemberID:   "member-root",
		SessionID:      "session-1",
		CWD:            "/scratch/project",
		WorkspaceCWD:   "/workspace/project",
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
	}
	if err := checkpoint.ValidateFor(expected); err != nil {
		t.Fatalf("ValidateFor: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ExecutorCheckpointExpectation)
	}{
		{name: "root", mutate: func(value *ExecutorCheckpointExpectation) { value.RootMemberID = "other-root" }},
		{name: "session", mutate: func(value *ExecutorCheckpointExpectation) { value.SessionID = "other-session" }},
		{name: "cwd", mutate: func(value *ExecutorCheckpointExpectation) { value.CWD = "/other/workspace" }},
		{name: "workspace", mutate: func(value *ExecutorCheckpointExpectation) { value.WorkspaceCWD = "/other/workspace" }},
		{name: "isolation", mutate: func(value *ExecutorCheckpointExpectation) { value.Isolated = true }},
		{name: "goal incarnation", mutate: func(value *ExecutorCheckpointExpectation) { value.GoalIncarnationID = "other-lease" }},
		{name: "provider", mutate: func(value *ExecutorCheckpointExpectation) {
			value.ModelSelection = checkpointSelection(t, "openai", "claude")
		}},
		{name: "model", mutate: func(value *ExecutorCheckpointExpectation) {
			value.ModelSelection = checkpointSelection(t, "anthropic", "claude-sonnet")
		}},
		{name: "limits", mutate: func(value *ExecutorCheckpointExpectation) {
			value.Limits = runfixture.MustLimits(run.LimitValues{MaxTotalTokens: runfixture.Pointer[int64](1)})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mismatch := expected
			test.mutate(&mismatch)
			if err := checkpoint.ValidateFor(mismatch); !errors.Is(err, ErrInvalidExecutorCheckpoint) {
				t.Fatalf("ValidateFor error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
		})
	}
}
