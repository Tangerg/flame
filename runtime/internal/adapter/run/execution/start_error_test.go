package execution

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
)

type unacknowledgedStartTrees struct {
	*testExecutionTrees
	cause   error
	attempt runs.ExecutionTreeHead
}

func (u *unacknowledgedStartTrees) SaveExecutionTree(_ context.Context, update runs.ExecutionTreeUpdate) error {
	u.attempt = update.Head
	return u.cause
}

func TestBeginRootPreservesScopeUnacknowledgedStartIdentity(t *testing.T) {
	cause := errors.New("initial tree commit unacknowledged")
	trees := &unacknowledgedStartTrees{testExecutionTrees: testTrees(t), cause: cause}
	calls := 0
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		calls++
		return interactionTextResponse("unexpected"), nil
	}), InteractionExecutorConfig{ExecutionTrees: trees})
	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := executor.Release(context.Background(), ref); err != nil {
			t.Error(err)
		}
	})
	if _, err := executor.Observe(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	err = executor.BeginRoot(t.Context(), ref)
	attempt, found := errors.AsType[*scoperun.RuntimeError](err)
	if !found || !errors.Is(err, cause) {
		t.Fatalf("start lost Scope's reconcilable evidence: %v", err)
	}
	expected := (scoperun.StartID{Scope: ref.SessionID, Key: ref.ExecutorID}).Root()
	if attempt.ProcessID() != expected || expected.String() != trees.attempt.RootID || attempt.Epoch() != 0 || attempt.After() != 0 {
		t.Fatalf("start evidence differs from the attempted head: %v, %+v", attempt, trees.attempt)
	}
	if calls != 0 {
		t.Fatal("an unacknowledged start dispatched provider work")
	}
}
