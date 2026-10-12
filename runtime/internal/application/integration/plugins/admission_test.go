package plugins

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// A change holds the serialization point across durable storage. Assembly
// registration must not queue behind it, and a party waiting for the point must
// still honor its own cancellation.
func TestInstallationAdmissionNeverQueuesAssemblyOrIgnoresCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		admission := NewAdmission()
		leave, err := admission.enter(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer leave()

		registered := make(chan struct{})
		go func() {
			_, finish := admission.Begin()
			finish()
			close(registered)
		}()
		synctest.Wait()
		select {
		case <-registered:
		default:
			t.Fatal("assembly registration waited behind a held change")
		}

		cause := errors.New("change abandoned")
		ctx, cancel := context.WithCancelCause(t.Context())
		waited := make(chan error, 1)
		go func() {
			_, err := admission.enter(ctx)
			waited <- err
		}()
		synctest.Wait()
		cancel(cause)
		synctest.Wait()
		if err := <-waited; !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
			t.Fatalf("waiting change = %v, want its own cancellation", err)
		}
	})
}

func TestAdmissionRefusesCanceledAndInactivePublication(t *testing.T) {
	admission := NewAdmission()
	cause := errors.New("command abandoned")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if err := admission.Under(ctx, func() error { t.Fatal("canceled command entered admission"); return nil }); !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission = %v", err)
	}
	other := NewAdmission()
	assembly, finish := other.Begin()
	defer finish()
	publish := func() error { t.Fatal("inactive assembly published"); return nil }
	if err := admission.Publish(t.Context(), assembly, nil, publish); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("foreign assembly = %v", err)
	}
	finish()
	if err := other.Publish(t.Context(), assembly, nil, publish); !errors.Is(err, plugin.ErrInvalid) {
		t.Fatalf("finished assembly = %v", err)
	}
}
