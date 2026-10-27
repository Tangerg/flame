package plugins

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/qimu"
	"testing"
	"testing/synctest"
)

// A change holds the serialization point across durable storage. Assembly
// registration must not queue behind it, and a party waiting for the point must
// still honor its own cancellation.
func TestInstallationAdmissionNeverQueuesAssemblyOrIgnoresCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		admission := qimu.NewAdmission()
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

		registered := make(chan struct{})
		go func() {
			_, finish, err := admission.Begin()
			if err != nil {
				t.Error(err)
			} else {
				finish()
			}
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
			waited <- admission.Under(ctx, func() error { t.Fatal("canceled waiter entered"); return nil })
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
	admission := qimu.NewAdmission()
	cause := errors.New("command abandoned")
	ctx, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	if err := admission.Under(ctx, func() error { t.Fatal("canceled command entered admission"); return nil }); !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission = %v", err)
	}
	other := qimu.NewAdmission()
	assembly, finish, err := other.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer finish()
	publish := func() error { t.Fatal("inactive assembly published"); return nil }
	if err := admission.Publish(t.Context(), assembly, nil, publish); !errors.Is(err, qimu.ErrInvalid) {
		t.Fatalf("foreign assembly = %v", err)
	}
	finish()
	if err := other.Publish(t.Context(), assembly, nil, publish); !errors.Is(err, qimu.ErrStaleBinding) {
		t.Fatalf("finished assembly = %v", err)
	}
}
