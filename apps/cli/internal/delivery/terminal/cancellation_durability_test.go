package terminal

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem/commit"
)

type unconfirmedCancellationStorage struct {
	*statefile.Store
	cause error
}

func (p *unconfirmedCancellationStorage) Replace(name string, body []byte) commit.Result {
	result := p.Store.Replace(name, body)
	if p.cause != nil && result.Committed() {
		result.State, result.Err = commit.Visible, p.cause
	}
	return result
}

type cancellationAdmissionProbe struct {
	*runtimefixture.Runtime
	calls int
}

func (r *cancellationAdmissionProbe) CancelRun(context.Context, conversation.CancelRun) (conversation.RunCancellation, error) {
	r.calls++
	return conversation.RunCancellation{}, conversation.ErrRunFinished
}

func TestCancellationDispatchWaitsForItsRetainedIdentityToBeDurable(t *testing.T) {
	storage, err := statefile.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fault := &unconfirmedCancellationStorage{Store: storage}
	store, err := workbench.Open(fault, workbench.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	profile := steerReplayTestProfile(t, t.TempDir())
	replay := testReplayPolicy(t, &profile)
	guard, err := replay.NewGuard()
	if err != nil {
		t.Fatal(err)
	}
	id := mutation.NewCommandID()
	if err := store.StagePendingRun(workbench.PendingRun{
		State: workbench.PendingRunQueued, Replay: authoring.UnprotectedGuard(), CancelReplay: authoring.UnprotectedGuard(),
		Command: authoring.StartRun{CommandID: id, SessionID: "session", Message: authoring.Message{Text: "original"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkPendingRunDispatching("session", id, guard, nil); err != nil {
		t.Fatal(err)
	}
	fault.cause = errors.New("cancellation directory sync failed")
	if _, err := store.MarkPendingRunCanceling("session", id, guard); !errors.Is(err, fault.cause) {
		t.Fatal(err)
	}
	pending := store.PendingRuns("session")[0]
	runtime := &cancellationAdmissionProbe{Runtime: runtimefixture.New()}
	application := &app{runtime: runtime, workbench: store, replayPolicy: replay}
	for _, dispatch := range []func(context.Context, conversation.CancelRun, authoring.ReplayGuard) error{application.cancelRootRun, application.cancelRuntimeNow} {
		request := conversation.CancelRun{CommandID: pending.CancelCommandID, RunID: "run_original", Reason: "original"}
		if err := dispatch(t.Context(), request, pending.CancelReplay); !errors.Is(err, fault.cause) || !mutation.OutcomeUnknown(err) {
			t.Fatalf("unconfirmed cancellation: %v", err)
		}
		if runtime.calls != 0 {
			t.Fatal("unconfirmed cancellation reached Runtime")
		}
	}
	if err := application.cancelRootRun(t.Context(), conversation.CancelRun{CommandID: mutation.NewCommandID(), RunID: "run_other"}, guard); err != nil || runtime.calls != 1 {
		t.Fatalf("independent live cancellation was blocked: %v, calls=%d", err, runtime.calls)
	}
}
