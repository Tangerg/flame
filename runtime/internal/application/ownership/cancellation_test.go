package ownership

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestCanceledForegroundOperationsPreserveLifetime(t *testing.T) {
	for _, operation := range []struct {
		name string
		run  func(*testing.T, *Gate, context.Context) error
	}{
		{"Session", func(t *testing.T, gate *Gate, ctx context.Context) error {
			release, acquired, err := gate.AcquireSession(ctx, "ses_1")
			if release != nil {
				release()
			}
			if acquired {
				t.Error("canceled request acquired the Session")
			}
			return err
		}},
		{"Run", func(t *testing.T, gate *Gate, ctx context.Context) error {
			admission, acquired, err := gate.AcquireRun(ctx, "ses_1", "/repo")
			admission.Release()
			if acquired {
				t.Error("canceled request acquired Run admission")
			}
			return err
		}},
		{"startability", func(_ *testing.T, gate *Gate, ctx context.Context) error {
			return gate.WaitRunStartable(ctx, "ses_1", "/repo")
		}},
	} {
		t.Run(operation.name, func(t *testing.T) {
			failure := errors.New("foreground request withdrawn")
			ctx, cancel := context.WithCancelCause(t.Context())
			cancel(failure)
			if err := operation.run(t, newTestGate(t), ctx); !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
				t.Fatalf("operation = %v, want cancellation and cause", err)
			}
		})
	}
}

func TestStartabilityCancellationPreventsContinuationAfterRelease(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gate := newTestGate(t)
		pending, acquired, err := gate.AcquireRun(t.Context(), "ses_1", "/repo")
		if err != nil || !acquired {
			t.Fatalf("pending admission = (%t, %v)", acquired, err)
		}
		defer pending.Release()
		failure := errors.New("continuation withdrawn")
		ctx, cancel := context.WithCancelCause(t.Context())
		defer cancel(nil)
		done := make(chan error, 1)
		go func() { done <- gate.WaitRunStartable(ctx, "ses_1", "/repo") }()
		synctest.Wait()
		select {
		case err := <-done:
			t.Fatalf("wait settled inside pending admission: %v", err)
		default:
		}
		cancel(failure)
		pending.Release()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) || !errors.Is(err, failure) {
			t.Fatalf("continuation = %v, want cancellation and cause", err)
		}
		live, acquired, err := gate.AcquireRun(t.Context(), "ses_1", "/repo")
		live.Release()
		if err != nil || !acquired {
			t.Fatalf("live request after cancellation = (%t, %v)", acquired, err)
		}
	})
}
