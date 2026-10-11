package runs

import (
	"context"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type canceledStartabilityReader struct {
	*fakeRunSessions
	cancel func()
	err    error
}

func (r *canceledStartabilityReader) ActiveRun(context.Context, string) (run.Run, bool, error) {
	r.cancel()
	return run.Run{}, false, r.err
}

func TestStartabilityRetainsCancellationDuringDurableRead(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "idle"},
		{name: "failed read", err: errors.New("durable Run unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := errors.New("continuation withdrawn")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			sessions := &canceledStartabilityReader{
				fakeRunSessions: &fakeRunSessions{sess: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")})},
				cancel:          func() { cancel(failure) },
				err:             test.err,
			}
			coordinator := newUseCaseCoordinator(&fakeExecutor{}, &fakeExecutionPorts{}, sessions, &fakeEffects{})
			err := coordinator.WaitSessionStartable(ctx, "ses_1")
			if !errors.Is(err, context.Canceled) || !errors.Is(err, failure) || test.err != nil && !errors.Is(err, test.err) {
				t.Fatalf("startability = %v, want complete canceled read outcome", err)
			}
		})
	}
}
