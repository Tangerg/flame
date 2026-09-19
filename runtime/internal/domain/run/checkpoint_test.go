package run_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func checkpointSelection(t *testing.T, provider, model string) modelref.Selection {
	t.Helper()
	selection, err := modelref.New(provider, model)
	if err != nil {
		t.Fatalf("modelref.New: %v", err)
	}
	return selection
}

func TestCheckpointConstructionValidatesHostEnvelope(t *testing.T) {
	valid := run.CheckpointState{
		RootMemberID: "root",
		Payload:      []byte(`{"executorOwned":"opaque"}`),
		BuildID:      testsupport.BuildID,
		Scope: run.ExecutionScope{
			SessionID:         "session-1",
			CWD:               "/workspace/project",
			Isolated:          true,
			GoalIncarnationID: "lease-1",
		},
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
		Limits: testsupport.MustRunLimits(run.LimitValues{
			MaxTotalTokens: testsupport.Pointer[int64](4_096),
			MaxBudgetUSD:   testsupport.Pointer(1.5),
			MaxSteps:       testsupport.Pointer(8),
		}),
		Usage: accounting.Snapshot{},
	}
	if _, err := run.NewCheckpoint(valid); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*run.CheckpointState)
	}{
		{name: "empty root", mutate: func(checkpoint *run.CheckpointState) { checkpoint.RootMemberID = "" }},
		{name: "unstable root", mutate: func(checkpoint *run.CheckpointState) { checkpoint.RootMemberID = " root" }},
		{name: "empty payload", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Payload = nil }},
		{name: "empty build", mutate: func(checkpoint *run.CheckpointState) { checkpoint.BuildID = "" }},
		{name: "empty model selection", mutate: func(checkpoint *run.CheckpointState) {
			checkpoint.ModelSelection = modelref.Selection{}
		}},
		{name: "unstable session", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Scope.SessionID = " session-1" }},
		{name: "unstable cwd", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Scope.CWD = "/workspace/project " }},
		{name: "goal incarnation whitespace", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Scope.GoalIncarnationID = "lease 1" }},
		{name: "goal incarnation non-printing", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Scope.GoalIncarnationID = "lease\u200b1" }},
		{name: "goal incarnation oversized", mutate: func(checkpoint *run.CheckpointState) {
			checkpoint.Scope.GoalIncarnationID = strings.Repeat("界", goalref.MaximumIncarnationCharacters+1)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkpoint := valid
			test.mutate(&checkpoint)
			if _, err := run.NewCheckpoint(checkpoint); !errors.Is(err, run.ErrInvalidCheckpoint) {
				t.Fatalf("Validate error = %v, want run.ErrInvalidCheckpoint", err)
			}
		})
	}
}

func TestCheckpointOwnsConstructionAndProjectionData(t *testing.T) {
	state := run.CheckpointState{
		RootMemberID: "root", Payload: []byte("payload"), BuildID: testsupport.BuildID,
		Scope: run.ExecutionScope{SessionID: "session"}, ModelSelection: checkpointSelection(t, "anthropic", "model"),
		Capabilities: run.Capabilities{InterruptKinds: []interrupt.Kind{interrupt.Question}},
		Usage:        accounting.Snapshot{Models: []accounting.ModelUsage{{Model: "model", Calls: 1}}},
	}
	checkpoint, err := run.NewCheckpoint(state)
	if err != nil {
		t.Fatal(err)
	}
	state.Payload[0] = 'X'
	state.Capabilities.InterruptKinds[0] = interrupt.Approval
	state.Usage.Models[0].Model = "changed"
	projection := checkpoint.State()
	projection.Payload[0] = 'Y'
	projection.Capabilities.InterruptKinds[0] = interrupt.Approval
	projection.Usage.Models[0].Model = "projection"
	if string(checkpoint.Payload()) != "payload" || checkpoint.Capabilities().InterruptKinds[0] != interrupt.Question || checkpoint.Usage().Models[0].Model != "model" {
		t.Fatal("checkpoint exposes mutable owned data")
	}
}

func TestCheckpointValidatesCrossAggregateOwnership(t *testing.T) {
	state := run.CheckpointState{
		RootMemberID: "member-root",
		Payload:      []byte("opaque"),
		BuildID:      testsupport.BuildID,
		Scope: run.ExecutionScope{
			SessionID:    "session-1",
			CWD:          "/scratch/project",
			WorkspaceCWD: "/workspace/project",
		},
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
	}
	checkpoint := testsupport.MustCheckpoint(state)
	expected := run.CheckpointExpectation{
		RootMemberID:   "member-root",
		SessionID:      "session-1",
		CWD:            "/scratch/project",
		WorkspaceCWD:   "/workspace/project",
		ModelSelection: checkpointSelection(t, "anthropic", "claude"),
	}
	if err := checkpoint.ValidateFor(expected); err != nil {
		t.Fatalf("ValidateFor: %v", err)
	}
	differentEffort := expected
	selectionWithEffort, err := modelref.NewWithReasoningEffort("anthropic", "claude", "high")
	if err != nil {
		t.Fatal(err)
	}
	differentEffort.ModelSelection = selectionWithEffort
	if err := checkpoint.ValidateFor(differentEffort); !errors.Is(err, run.ErrInvalidCheckpoint) ||
		!strings.Contains(err.Error(), "reasoning effort high") {
		t.Fatalf("reasoning mismatch error = %v, want complete selection identity", err)
	}

	tests := []struct {
		name   string
		mutate func(*run.CheckpointExpectation)
	}{
		{name: "root", mutate: func(value *run.CheckpointExpectation) { value.RootMemberID = "other-root" }},
		{name: "session", mutate: func(value *run.CheckpointExpectation) { value.SessionID = "other-session" }},
		{name: "cwd", mutate: func(value *run.CheckpointExpectation) { value.CWD = "/other/workspace" }},
		{name: "workspace", mutate: func(value *run.CheckpointExpectation) { value.WorkspaceCWD = "/other/workspace" }},
		{name: "isolation", mutate: func(value *run.CheckpointExpectation) { value.Isolated = true }},
		{name: "goal incarnation", mutate: func(value *run.CheckpointExpectation) { value.GoalIncarnationID = "other-lease" }},
		{name: "provider", mutate: func(value *run.CheckpointExpectation) {
			value.ModelSelection = checkpointSelection(t, "openai", "claude")
		}},
		{name: "model", mutate: func(value *run.CheckpointExpectation) {
			value.ModelSelection = checkpointSelection(t, "anthropic", "claude-sonnet")
		}},
		{name: "empty model selection", mutate: func(value *run.CheckpointExpectation) {
			value.ModelSelection = modelref.Selection{}
		}},
		{name: "limits", mutate: func(value *run.CheckpointExpectation) {
			value.Limits = testsupport.MustRunLimits(run.LimitValues{MaxTotalTokens: testsupport.Pointer[int64](1)})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mismatch := expected
			test.mutate(&mismatch)
			if err := checkpoint.ValidateFor(mismatch); !errors.Is(err, run.ErrInvalidCheckpoint) {
				t.Fatalf("ValidateFor error = %v, want run.ErrInvalidCheckpoint", err)
			}
		})
	}
}

func TestCheckpointRequiresOneExecutionAndMonotonicUsage(t *testing.T) {
	state := run.CheckpointState{
		RootMemberID: "root", Payload: []byte("first"), BuildID: testsupport.BuildID,
		Scope:          run.ExecutionScope{SessionID: "session", CWD: "/project", WorkspaceCWD: "/project"},
		ModelSelection: checkpointSelection(t, "anthropic", "model"),
		Usage:          accounting.Snapshot{Models: []accounting.ModelUsage{{Model: "model", Calls: 2}}},
	}
	checkpoint := testsupport.MustCheckpoint(state)
	advanced := checkpoint.State()
	advanced.Payload = []byte("second")
	advanced.Usage.Models[0].Calls = 3
	next := testsupport.MustCheckpoint(advanced)
	if err := checkpoint.ValidateSuccessor(next); err != nil {
		t.Fatalf("advance: %v", err)
	}
	if err := next.ValidateSuccessor(checkpoint); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("usage regression: %v", err)
	}
	for name, mutate := range map[string]func(*run.CheckpointState){
		"root":      func(state *run.CheckpointState) { state.RootMemberID = "other" },
		"session":   func(state *run.CheckpointState) { state.Scope.SessionID = "other" },
		"build":     func(state *run.CheckpointState) { state.BuildID = testsupport.AlternateBuildID },
		"workspace": func(state *run.CheckpointState) { state.Scope.WorkspaceCWD = "/other" },
		"model":     func(state *run.CheckpointState) { state.ModelSelection = checkpointSelection(t, "openai", "model") },
		"limits": func(state *run.CheckpointState) {
			state.Limits = testsupport.MustRunLimits(run.LimitValues{MaxSteps: testsupport.Pointer(1)})
		},
		"capabilities": func(state *run.CheckpointState) { state.Capabilities.ChildRuns = true },
	} {
		t.Run(name, func(t *testing.T) {
			changed := checkpoint.State()
			mutate(&changed)
			if err := checkpoint.ValidateSuccessor(testsupport.MustCheckpoint(changed)); !errors.Is(err, run.ErrInvalidCheckpoint) {
				t.Fatalf("changed execution policy: %v", err)
			}
		})
	}
	if err := checkpoint.ValidateSuccessor(run.Checkpoint{}); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("zero successor: %v", err)
	}
	if err := (run.Checkpoint{}).ValidateSuccessor(checkpoint); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("zero predecessor: %v", err)
	}
	if err := (run.Checkpoint{}).ValidateOwnership("root", "session"); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("zero ownership: %v", err)
	}
}
