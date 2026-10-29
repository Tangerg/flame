package runs

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
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

type settlingStartabilitySessions struct{ *fakeRunSessions }

func (s *settlingStartabilitySessions) ApplyRunCancel(ctx context.Context, sessionID, runID, reason string, at time.Time) (run.Run, error) {
	terminal, err := s.fakeRunSessions.ApplyRunCancel(ctx, sessionID, runID, reason, at)
	if err == nil {
		s.setActive(nil)
	}
	return terminal, err
}

func TestParkedCancellationWakesSessionStartability(t *testing.T) {
	for _, cleanupError := range []error{nil, errors.New("executor retirement failed")} {
		synctest.Test(t, func(t *testing.T) {
			pending := testApprovalPending("member_waiting", time.Now().UTC())
			waiting := runForPending(pending)
			sessions := &settlingStartabilitySessions{fakeRunSessions: &fakeRunSessions{
				sess:   testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")}),
				active: &waiting, pending: map[string]Pending{"run_1": pending},
			}}
			coordinator := mustNewCoordinator(Dependencies{
				Releases: &fakeExecutionPorts{releaseErr: cleanupError}, Session: testSessionPorts(sessions),
				Runs: &fakeRunProjection{runs: map[string]run.Run{"run_1": waiting}}, Admissions: testsupport.NewAdmissionGate(),
			})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- coordinator.WaitSessionStartable(ctx, "ses_1") }()
			synctest.Wait()
			other, ok, err := coordinator.admission.AcquireSession(t.Context(), "ses_other")
			if err != nil || !ok {
				t.Fatalf("unrelated admission = %t, %v", ok, err)
			}
			other()
			synctest.Wait()
			select {
			case err := <-done:
				t.Fatalf("unrelated release bypassed the waiting Run: %v", err)
			default:
			}
			_, err = coordinator.Cancel(t.Context(), CancelCommand{RunID: "run_1", Reason: "stop waiting"})
			if !errors.Is(err, cleanupError) {
				t.Fatalf("parked cancellation = %v, want %v", err, cleanupError)
			}
			synctest.Wait()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatal("committed parked cancellation did not wake the Session waiter")
			}
		})
	}
}

type settlingStartabilityRecovery struct {
	*recoveryStoreStub
	sessions *fakeRunSessions
}

func (s *settlingStartabilityRecovery) CommitRecovery(ctx context.Context, commit RecoveryCommit) error {
	if err := s.recoveryStoreStub.CommitRecovery(ctx, commit); err != nil {
		return err
	}
	if len(commit.LostRuns()) > 0 {
		s.sessions.setActive(nil)
	}
	return nil
}

func TestSurvivorRecoveryWakesExistingSessionWaiterAfterCommit(t *testing.T) {
	for _, commitError := range []error{nil, errors.New("recovery transaction refused")} {
		synctest.Test(t, func(t *testing.T) {
			orphan := testsupport.MustRestoreRun(run.Snapshot{ID: "run_orphan", SessionID: "ses_1", State: run.Running})
			sessions := &fakeRunSessions{
				sess: testsupport.MustRestoreSession(session.Snapshot{ID: "ses_1", Workspace: testsupport.MustWorkspace("/work")}), active: &orphan,
			}
			coordinator := newUseCaseCoordinator(&fakeExecutor{}, &fakeExecutionPorts{}, sessions, &fakeEffects{})
			store := &settlingStartabilityRecovery{
				recoveryStoreStub: &recoveryStoreStub{runs: []run.Run{orphan}, transcripts: map[string][]transcript.Item{}, messageMarks: map[string]int{}, commitErr: commitError},
				sessions:          sessions,
			}
			recovery, err := NewRecovery(store, waitingExecutionResumabilityFunc(func(context.Context, ExecutionContinuation) (bool, error) { return false, nil }), coordinator.admission, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- coordinator.WaitSessionStartable(ctx, "ses_1") }()
			synctest.Wait()
			recovered, err := recovery.Reconcile(t.Context())
			if !errors.Is(err, commitError) || commitError == nil && recovered != 1 {
				t.Fatalf("recovery = %d, %v; want commit outcome %v", recovered, err, commitError)
			}
			synctest.Wait()
			if commitError != nil {
				select {
				case err := <-done:
					t.Fatalf("refused recovery made an active Session startable: %v", err)
				default:
				}
				cancel()
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("wait cancellation = %v", err)
				}
				return
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			default:
				t.Fatal("committed survivor recovery did not wake the existing waiter")
			}
		})
	}
}
