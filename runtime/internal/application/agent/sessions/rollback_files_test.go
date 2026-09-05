package sessions

import (
	"context"
	"testing"
)

func TestRollbackSpecOwnsClosedRestoreScope(t *testing.T) {
	valid := []struct {
		name    string
		scope   RestoreScope
		files   bool
		history bool
	}{
		{name: "history", scope: RestoreHistory, history: true},
		{name: "files", scope: RestoreFiles, files: true},
		{name: "both", scope: RestoreBoth, files: true, history: true},
	}
	for _, test := range valid {
		t.Run(test.name, func(t *testing.T) {
			spec := RollbackSpec{SessionID: "ses_1", ToRunID: "run_1", Scope: test.scope}
			if err := spec.validate(); err != nil {
				t.Fatalf("validate: %v", err)
			}
			if got := test.scope.RestoresFiles(); got != test.files {
				t.Fatalf("RestoresFiles = %v, want %v", got, test.files)
			}
			if got := test.scope.RestoresHistory(); got != test.history {
				t.Fatalf("RestoresHistory = %v, want %v", got, test.history)
			}
		})
	}
	if err := (RollbackSpec{SessionID: "ses_1", Scope: RestoreHistory}).validate(); err != nil {
		t.Fatalf("history rollback to an empty boundary: %v", err)
	}

	invalid := []RollbackSpec{
		{SessionID: "ses_1"},
		{SessionID: "ses_1", Scope: RestoreScope("workspace")},
		{SessionID: "ses_1", Scope: RestoreFiles},
		{SessionID: "ses_1", Scope: RestoreBoth},
	}
	for _, spec := range invalid {
		if err := spec.validate(); err == nil {
			t.Fatalf("validate(%+v) succeeded", spec)
		}
	}
}

func TestMutationCompletionDetachesFromCallerCancellation(t *testing.T) {
	mutations := new(observingMutations)
	coordinator := mustNewCoordinator(Dependencies{Mutations: mutations})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := coordinator.completeMutationDetached(ctx, "ses_1"); err != nil {
		t.Fatalf("completeMutationDetached: %v", err)
	}
	if mutations.canceled {
		t.Fatal("mutation cleanup inherited caller cancellation")
	}
	if !mutations.bounded {
		t.Fatal("mutation cleanup context has no deadline")
	}
}

type observingMutations struct {
	canceled bool
	bounded  bool
}

func (*observingMutations) Record(context.Context, WorkspaceMutation) error { return nil }

func (o *observingMutations) Complete(ctx context.Context, _ string) error {
	o.canceled = ctx.Err() != nil
	_, o.bounded = ctx.Deadline()
	return nil
}

func (*observingMutations) ListPending(context.Context) ([]WorkspaceMutation, error) {
	return nil, nil
}
