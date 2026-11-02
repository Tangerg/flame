package goals

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/goalworkflow"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport/goaltest"
	"go.opentelemetry.io/otel/trace"
)

type alreadyTerminalRuns struct{}

func (alreadyTerminalRuns) WaitSessionStartable(context.Context, string) error          { return nil }
func (alreadyTerminalRuns) AdmitSelection(context.Context, modelchoice.Selection) error { return nil }
func (alreadyTerminalRuns) Start(context.Context, runs.StartCommand) (runs.StartResult, error) {
	return runs.StartResult{}, nil
}
func (alreadyTerminalRuns) Cancel(context.Context, runs.CancelCommand) (runs.CancelResult, error) {
	return runs.CancelResult{}, runs.ErrRunFinished
}

func TestSessionCommandLocksDoNotCoupleUnrelatedSessions(t *testing.T) {
	mutations := NewSessionMutations()
	releaseFirst := mustAcquireSessionCommand(t, t.Context(), mutations, "ses_1")

	unrelatedAcquired := make(chan struct{})
	releaseUnrelated := make(chan struct{})
	unrelatedDone := make(chan struct{})
	go func() {
		defer close(unrelatedDone)
		release := mustAcquireSessionCommand(t, t.Context(), mutations, "ses_2")
		close(unrelatedAcquired)
		<-releaseUnrelated
		release()
	}()
	select {
	case <-unrelatedAcquired:
	case <-time.After(time.Second):
		t.Fatal("unrelated session command was blocked")
	}

	sameAcquired := make(chan struct{})
	releaseSame := make(chan struct{})
	sameDone := make(chan struct{})
	go func() {
		defer close(sameDone)
		release := mustAcquireSessionCommand(t, t.Context(), mutations, "ses_1")
		close(sameAcquired)
		<-releaseSame
		release()
	}()
	select {
	case <-sameAcquired:
		t.Fatal("same-session command crossed the ownership lock")
	default:
	}

	releaseFirst()
	select {
	case <-sameAcquired:
	case <-time.After(time.Second):
		t.Fatal("same-session command did not continue after release")
	}
	close(releaseSame)
	close(releaseUnrelated)
	<-sameDone
	<-unrelatedDone
}

func mustAcquireSessionCommand(
	t *testing.T,
	ctx context.Context,
	mutations *SessionMutations,
	sessionID string,
) func() {
	t.Helper()
	release, err := mutations.acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("acquire %s: %v", sessionID, err)
	}
	return release
}

// TestSessionCommandWaitEndsWithItsOwnRequest: the holder owns a whole durable
// write-set, so a command queued behind it leaves when its own request does
// instead of outliving it inside an uninterruptible wait.
func TestSessionCommandWaitEndsWithItsOwnRequest(t *testing.T) {
	mutations := NewSessionMutations()
	release := mustAcquireSessionCommand(t, t.Context(), mutations, "ses_1")
	defer release()

	waiting, cancelWaiting := context.WithCancel(t.Context())
	queued := make(chan error, 1)
	go func() {
		_, err := mutations.acquire(waiting, "ses_1")
		queued <- err
	}()
	select {
	case err := <-queued:
		t.Fatalf("queued command crossed a held session lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	cancelWaiting()
	select {
	case err := <-queued:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued command error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued command outlived its own request")
	}
}

func TestSessionMutationQuiescesEveryGoalBeforeJoining(t *testing.T) {
	firstErr := errors.New("first goal cleanup failed")
	first := completedGoalDrive(firstErr)
	second := completedGoalDrive(nil)
	var firstCanceled atomic.Bool
	var secondCanceled atomic.Bool
	first.cancel = func() { firstCanceled.Store(true) }
	second.cancel = func() { secondCanceled.Store(true) }

	mutations := NewSessionMutations()
	mutations.drives["ses_1"] = first
	mutations.drives["ses_2"] = second

	// The commit succeeded, so a drive that will not quiesce settles: reporting it
	// would tell the caller a mutation that happened did not.
	err := mutations.WithSessionMutation(
		t.Context(),
		[]string{"ses_1", "ses_2"},
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
	)
	if err != nil {
		t.Fatalf("WithSessionMutation reported settlement as a failed commit: %v", err)
	}
	if !firstCanceled.Load() || !secondCanceled.Load() {
		t.Fatalf("quiesced goals = first:%v second:%v, want both", firstCanceled.Load(), secondCanceled.Load())
	}
	if mutations.activeDrive("ses_1") != nil || mutations.activeDrive("ses_2") != nil {
		t.Fatal("successful session mutation retained a goal driver")
	}
}

func TestQuiesceRetainsJoinIdentityUntilDriverExits(t *testing.T) {
	mutations := NewSessionMutations()
	var canceled atomic.Bool
	drive := &goalDrive{
		incarnationID: "lease_1",
		cancel:        func() { canceled.Store(true) },
		done:          make(chan struct{}),
	}
	mutations.drives["ses_1"] = drive
	driver := &Driver{mutations: mutations}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := driver.quiesceDrive(ctx, "ses_1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("quiesceDrive error = %v, want canceled wait", err)
	}
	if !canceled.Load() {
		t.Fatal("quiesce did not cancel the driver")
	}
	if got := mutations.activeDrive("ses_1"); got != drive {
		t.Fatalf("drive after timed-out join = %p, want retained drive %p", got, drive)
	}

	mutations.forget("ses_1", drive)
	close(drive.done)
	if got := mutations.activeDrive("ses_1"); got != nil {
		t.Fatalf("drive after owner exit = %p, want released", got)
	}
}

func TestGoalDriveAwaitReturnsCompletedOutcomeAfterCallerCancellation(t *testing.T) {
	want := errors.New("driver failed")
	drive := completedGoalDrive(want)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := drive.await(ctx); !errors.Is(err, want) {
		t.Fatalf("await() = %v, want completed outcome", err)
	}
}

func TestGoalRunCleanupAcceptsAnAlreadyTerminalRun(t *testing.T) {
	driver := &Driver{cancellations: alreadyTerminalRuns{}}

	if err := driver.cancelRun(t.Context(), "run_1"); err != nil {
		t.Fatalf("cancelRun after terminal commit: %v", err)
	}
}

func completedGoalDrive(err error) *goalDrive {
	done := make(chan struct{})
	close(done)
	return &goalDrive{
		done:   done,
		err:    err,
		cancel: func() {},
	}
}

func TestDriveFailureRequiresOnlyItsOwnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	failure := errors.New("storage failed")
	tests := []struct {
		name     string
		ctx      context.Context
		err      error
		canceled bool
	}{
		{name: "another_lifetime", ctx: t.Context(), err: context.Canceled},
		{name: "wrapped_cancellation", ctx: ctx, err: fmt.Errorf("read: %w", context.Canceled), canceled: true},
		{name: "joined_cancellations", ctx: ctx, err: errors.Join(context.Canceled, fmt.Errorf("read: %w", context.Canceled)), canceled: true},
		{name: "joined_storage_failure", ctx: ctx, err: fmt.Errorf("read: %w", errors.Join(context.Canceled, failure))},
		{name: "independent_deadline", ctx: ctx, err: errors.Join(context.Canceled, context.DeadlineExceeded)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := driveFailure(test.ctx, test.err)
			if test.canceled {
				if got != nil {
					t.Fatalf("quiescence error = %v", got)
				}
			} else if got != test.err {
				t.Fatalf("failure = %v, want original %v", got, test.err)
			}
		})
	}
}

func TestGoalDriveAwaitPreservesPendingCallerCancellationCause(t *testing.T) {
	cause := errors.New("request abandoned")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	drive := &goalDrive{done: make(chan struct{})}
	if err := drive.await(ctx); !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
		t.Fatalf("await = %v, want caller cancellation and original cause", err)
	}
}

type canceledPauseWorkflow struct {
	goaltest.Workflow
	cancel context.CancelFunc
}

func (w canceledPauseWorkflow) DecideGoal(ctx context.Context, _ extension.ContributionSource, _ goalworkflow.Request) (goalworkflow.AuthoredDecision, error) {
	w.cancel()
	return goalworkflow.AuthoredDecision{}, ctx.Err()
}

func TestTerminalFaultSurvivesCanceledPause(t *testing.T) {
	for _, test := range []struct {
		name     string
		finished *run.Run
	}{
		{name: "missing_boundary"},
		{name: "malformed_boundary", finished: &run.Run{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			current, err := goaltest.New("s1", "continue", modelchoice.Selection{}, run.Capabilities{}, "incarnation", time.Now())
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			driver := &Driver{workflow: canceledPauseWorkflow{cancel: cancel}, now: time.Now}
			_, err = driver.resolveTerminalRun(ctx, &current, trace.SpanFromContext(ctx), test.finished)
			if !errors.Is(err, errTerminalOutcomeMissing) || !errors.Is(err, context.Canceled) {
				t.Fatalf("resolution = %v, want original terminal fault and canceled pause", err)
			}
		})
	}
}
