package mcp

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"

	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

func TestRetirementWaitCancellationPreservesOwnedResult(t *testing.T) {
	for _, operation := range []struct {
		name string
		join func(*Connections, context.Context, *sdkmcp.ClientSession) error
	}{
		{"shutdown", func(c *Connections, ctx context.Context, _ *sdkmcp.ClientSession) error {
			return c.Shutdown(ctx)
		}},
		{"session", func(c *Connections, ctx context.Context, session *sdkmcp.ClientSession) error {
			return c.closeSession(ctx, session)
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				closeFailure := errors.New("transport retirement failed")
				waitFailure := errors.New("caller stopped waiting")
				release := make(chan struct{})
				finish := sync.OnceFunc(func() { close(release) })
				defer finish()
				var calls atomic.Int32
				session := new(sdkmcp.ClientSession)
				c := &Connections{
					lifetime: t.Context(),
					sessions: map[*sdkmcp.ClientSession]*ownedSession{session: {closeFn: func() error {
						calls.Add(1)
						<-release
						return closeFailure
					}}},
				}
				ctx, cancel := context.WithCancelCause(t.Context())
				defer cancel(nil)
				done := make(chan error, 1)
				go func() { done <- operation.join(c, ctx, session) }()
				synctest.Wait()
				cancel(waitFailure)
				synctest.Wait()
				if err := <-done; !errors.Is(err, context.Canceled) || !errors.Is(err, waitFailure) {
					t.Errorf("canceled wait = %v, want cancellation and caller cause", err)
				}
				finish()
				if err := c.Shutdown(t.Context()); !errors.Is(err, closeFailure) {
					t.Errorf("shutdown = %v, want retirement failure", err)
				}
				if err := c.Shutdown(ctx); !errors.Is(err, closeFailure) || errors.Is(err, waitFailure) {
					t.Errorf("settled shutdown = %v, want owned result without waiter cancellation", err)
				}
				if got := calls.Load(); got != 1 {
					t.Errorf("cleanup calls = %d, want one", got)
				}
			})
		})
	}
}
