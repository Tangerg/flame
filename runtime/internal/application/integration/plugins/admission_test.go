package plugins

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/frame"
)

// A publication waiting for a durable change keeps its own cancellation; it
// cannot enter later using a request whose caller has already left.
func TestInstallationAdmissionHonorsCancellationWhileAChangeOwnsTheGate(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		admission := frame.NewAdmission()
		entered, release := make(chan struct{}), make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			finished <- admission.Under(t.Context(), func() error { close(entered); <-release; return nil })
		}()
		<-entered
		defer func() {
			close(release)
			if err := <-finished; err != nil {
				t.Error(err)
			}
		}()

		cause := errors.New("change abandoned")
		ctx, cancel := context.WithCancelCause(t.Context())
		waited := make(chan error, 1)
		waiterEntered := make(chan struct{})
		go func() {
			waited <- admission.Under(ctx, func() error { close(waiterEntered); return nil })
		}()
		synctest.Wait()
		cancel(cause)
		synctest.Wait()
		err := <-waited
		select {
		case <-waiterEntered:
			t.Fatal("canceled waiter entered")
		default:
		}
		if !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting change = %v, want its own cancellation", err)
		}
	})
}

func TestAdmissionRefusesCanceledAndInactivePublication(t *testing.T) {
	admission := frame.NewAdmission()
	cause := errors.New("command abandoned")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if err := admission.Under(ctx, func() error { t.Fatal("canceled command entered admission"); return nil }); !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission = %v", err)
	}
	host := testFramework(t, nil)
	validate := func(ctx context.Context) error {
		return ValidateExecutionSourcesUnder(ctx, host, func(context.Context, plugin.Activation) error {
			t.Fatal("empty publication resolved a binding")
			return nil
		}, nil, nil)
	}
	if err := admission.Admit(t.Context(), validate); !errors.Is(err, frame.ErrInvalid) {
		t.Fatalf("foreign admission = %v", err)
	}
	var escaped context.Context
	if err := host.Admission().Admit(t.Context(), func(ctx context.Context) error { escaped = ctx; return validate(ctx) }); err != nil {
		t.Fatal(err)
	}
	if err := validate(escaped); !errors.Is(err, frame.ErrInvalid) {
		t.Fatalf("expired admission = %v", err)
	}
}
