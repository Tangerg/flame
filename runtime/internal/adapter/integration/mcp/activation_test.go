package mcp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type failedDefinitionRegistry struct {
	sourceRegistry
	err error
}

func (r failedDefinitionRegistry) Definition(context.Context, mcpserver.ID) (mcpserver.Server, bool, error) {
	return mcpserver.Server{}, false, r.err
}

func TestInstallationActivationPreservesDefinitionFailures(t *testing.T) {
	origin, err := mcpserver.InstallationOrigin(testsupport.InstallationID(t))
	if err != nil {
		t.Fatal(err)
	}
	id, err := mcpserver.NewID(origin, testsupport.ServerName("workflow"))
	if err != nil {
		t.Fatal(err)
	}
	storageFailure := errors.New("installation read failed")
	canceled, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("activation request withdrawn")
	cancel(cause)
	cancellations := errors.Join(canceled.Err(), context.Cause(canceled))
	for _, operation := range []struct {
		name string
		call func(*Pool, context.Context, mcpserver.ID) (plugin.Activation, error)
	}{
		{name: "current", call: (*Pool).InstallationActivation},
		{name: "await", call: (*Pool).AwaitInstallationActivation},
	} {
		for _, failure := range []error{storageFailure, cancellations, errors.Join(storageFailure, cancellations)} {
			t.Run(operation.name+"/"+failure.Error(), func(t *testing.T) {
				pool := &Pool{registry: failedDefinitionRegistry{err: failure}}
				_, err := operation.call(pool, t.Context(), id)
				if err != failure || errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, ErrUnavailable) {
					t.Fatalf("definition failure = %v, want original %v without source-loss classification", err, failure)
				}
			})
		}
		t.Run(operation.name+"/missing", func(t *testing.T) {
			pool := &Pool{registry: failedDefinitionRegistry{}}
			if _, err := operation.call(pool, t.Context(), id); !errors.Is(err, plugin.ErrUnavailable) || !errors.Is(err, ErrUnavailable) {
				t.Fatalf("missing source = %v, want unavailable", err)
			}
		})
	}
}

func TestWaitSessionPreservesCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("wait withdrawn")
	cancel(cause)
	owner := &connections{lifetime: t.Context()}
	session, err := owner.waitSession(ctx, mcpserver.Server{})
	if session != nil || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("canceled activation wait = %v, %v, want only caller cancellation", session, err)
	}
}

func TestBorrowSessionJoinsOriginalStartupAttempt(t *testing.T) {
	for _, test := range []struct {
		name           string
		cancelBorrower bool
	}{
		{name: "source failure"},
		{name: "caller cancellation", cancelBorrower: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				config := startupDescriptor("pending")
				config.Server.Enabled = true
				owner := &connections{lifetime: t.Context()}
				pool := &Pool{inner: owner, registry: &mutableSourceRegistry{server: config.Server}}
				entered, release := make(chan struct{}), make(chan struct{})
				finish := sync.OnceFunc(func() { close(release) })
				defer finish()
				t.Cleanup(func() { _ = owner.shutdown(t.Context()) })
				failure := errors.New("original source refused startup")
				attempt, err := owner.beginConnect(t.Context(), config.Server.ID(), func(context.Context, mcpserver.ID) (*launchPermit, error) {
					close(entered)
					<-release
					return nil, failure
				})
				if err != nil {
					t.Fatal(err)
				}
				completed := make(chan error, 1)
				go func() { completed <- attempt.Run() }()
				<-entered
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				borrowed := make(chan error, 1)
				go func() {
					session, err := pool.BorrowSession(ctx, config.Server.ID())
					if session != nil {
						_ = session.Close()
					}
					borrowed <- err
				}()
				synctest.Wait()
				select {
				case err := <-borrowed:
					t.Fatalf("pending source was treated as a completed connection: %v", err)
				default:
				}
				if test.cancelBorrower {
					cause := errors.New("caller stopped waiting")
					cancel(cause)
					if err := <-borrowed; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || errors.Is(err, ErrUnavailable) {
						t.Fatalf("borrow cancellation = %v", err)
					}
					synctest.Wait()
					select {
					case err := <-completed:
						t.Fatalf("borrower ended the original startup attempt: %v", err)
					default:
					}
				}
				finish()
				if err := <-completed; !errors.Is(err, failure) {
					t.Fatalf("startup outcome = %v", err)
				}
				if !test.cancelBorrower {
					if err := <-borrowed; !errors.Is(err, failure) || !errors.Is(err, ErrUnavailable) {
						t.Fatalf("borrow lost the original startup failure: %v", err)
					}
				}
			})
		})
	}
}
