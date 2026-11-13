package bootstrap

import (
	"context"
	"errors"
	"os"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/frame"
	"github.com/Tangerg/frame/process"
	"github.com/Tangerg/frame/process/runner"
	"github.com/Tangerg/frame/value"
)

func TestProcessCreationJoinsOriginalLaunchAfterCancellation(t *testing.T) {
	transportFailure := errors.New("original runner response failed")
	for _, failure := range []error{nil, transportFailure} {
		synctest.Test(t, func(t *testing.T) {
			type launchOutcome struct {
				launch runner.LaunchResult
				err    error
			}
			type traceKey struct{}
			ctx, cancel := context.WithCancelCause(context.WithValue(t.Context(), traceKey{}, "original trace"))
			defer cancel(nil)
			request := creationRequest()
			original := runner.LaunchResult{Observation: runner.Observation{
				AttemptID: request.AttemptID, Epoch: request.Epoch, RunnerID: request.RunnerID,
				State: runner.Running, PID: os.Getpid(),
			}}
			entered, complete := make(chan struct{}), make(chan struct{})
			calls := 0
			next := creationRunner{launch: func(callCtx context.Context, received runner.LaunchRequest) (runner.LaunchResult, error) {
				calls++
				if received.AttemptRef != request.AttemptRef || callCtx.Value(traceKey{}) != "original trace" {
					t.Error("original request identity or trace changed")
				}
				close(entered)
				<-complete
				if err := callCtx.Err(); err != nil {
					return runner.LaunchResult{}, err
				}
				return original, failure
			}}
			result := make(chan launchOutcome, 1)
			go func() {
				launch, err := (processRunner{next}).Launch(ctx, request)
				result <- launchOutcome{launch, err}
			}()
			<-entered
			cancel(errors.New("host is retiring"))
			synctest.Wait()
			select {
			case <-result:
				t.Fatal("creation returned before its original result")
			default:
			}
			close(complete)
			observed := <-result
			if observed.launch.Observation != original.Observation || !errors.Is(observed.err, failure) || calls != 1 {
				t.Errorf("creation = %+v, error = %v, calls = %d; want original result and failure from one invocation", observed.launch, observed.err, calls)
			}
		})
	}
}

func TestProcessCreationPreservesOriginalNonDispatchProof(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	authority, err := runner.NewDaemon(runner.DaemonConfig{Executable: executable, Directory: t.TempDir(), StartTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	cause := errors.New("creation was canceled before dispatch")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	for _, callCtx := range []context.Context{ctx, nil} {
		request := creationRequest()
		result, err := (processRunner{authority}).Launch(callCtx, request)
		if !result.NotDispatched(request) {
			t.Error("original non-dispatch proof was lost")
		}
		if callCtx == nil {
			if !errors.Is(err, frame.ErrInvalid) {
				t.Errorf("nil context error = %v, want invalid context", err)
			}
		} else if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Errorf("canceled context error = %v, want cancellation and original cause", err)
		}
	}
}

func creationRequest() runner.LaunchRequest {
	return runner.LaunchRequest{
		AttemptRef: runner.AttemptRef{AttemptID: "original-attempt", Epoch: 1, RunnerID: "original-runner"},
		Command:    "/original/executable", ExecutableDigest: value.Hash([]byte("original executable")),
		Configuration: value.Hash([]byte("original configuration")),
	}
}

type creationRunner struct {
	process.Runner
	launch func(context.Context, runner.LaunchRequest) (runner.LaunchResult, error)
}

func (r creationRunner) Launch(ctx context.Context, request runner.LaunchRequest) (runner.LaunchResult, error) {
	return r.launch(ctx, request)
}
