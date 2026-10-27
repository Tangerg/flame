package qimu_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/qimu"
)

func TestUnconfirmedConsumerPinsProviderAndLateProofResumesOriginalClose(t *testing.T) {
	ctx := testContext(t)
	lifetime := qimu.NewLifetime(context.Background())
	var providerClosed, consumerClosed, independentClosed atomic.Int64
	provider, err := lifetime.Own(qimu.Resource{Name: "provider", Close: func(context.Context) (qimu.ReleaseState, error) {
		providerClosed.Add(1)
		return qimu.ResourceReleased, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := lifetime.Own(qimu.Resource{Name: "consumer", Close: func(context.Context) (qimu.ReleaseState, error) {
		consumerClosed.Add(1)
		return qimu.ResourceUnconfirmed, errors.New("connection lost")
	}}, provider)
	if err != nil {
		t.Fatal(err)
	}
	_, err = lifetime.Own(qimu.Resource{Name: "independent", Close: func(context.Context) (qimu.ReleaseState, error) {
		independentClosed.Add(1)
		return qimu.ResourceReleased, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	closing := lifetime.BeginClose(qimu.ErrClosed)
	first, err := closing.Wait(ctx)
	if !errors.Is(err, qimu.ErrUnconfirmedRelease) || first.Released {
		t.Fatalf("unknown release became final: %+v %v", first, err)
	}
	if providerClosed.Load() != 0 || consumerClosed.Load() != 1 || independentClosed.Load() != 1 {
		t.Fatalf("incorrect close order: %d %d %d", providerClosed.Load(), consumerClosed.Load(), independentClosed.Load())
	}
	if err := consumer.ConfirmReleased(nil); err != nil {
		t.Fatal(err)
	}
	final, err := closing.WaitReleased(ctx)
	if !final.Released || final.ID != first.ID || providerClosed.Load() != 1 || consumerClosed.Load() != 1 {
		t.Fatalf("late proof restarted or skipped original close: %+v %v", final, err)
	}
}

func TestCanceledWaiterDoesNotCancelOwnerAndLateAcquisitionIsCanceled(t *testing.T) {
	ctx := testContext(t)
	lifetime := qimu.NewLifetime(context.Background())
	work, err := lifetime.Enter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	closing := lifetime.BeginClose(errors.New("owner shutdown"))
	waitCtx, cancel := context.WithCancel(ctx)
	cancel()
	if outcome, err := closing.Wait(waitCtx); outcome.Released || !errors.Is(err, context.Canceled) {
		t.Fatalf("wait result: %+v %v", outcome, err)
	}
	var canceled, closed atomic.Int64
	_, err = work.Own(qimu.Resource{Name: "late", Cancel: func(error) { canceled.Add(1) }, Close: func(context.Context) (qimu.ReleaseState, error) { closed.Add(1); return qimu.ResourceReleased, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if canceled.Load() != 1 {
		t.Fatal("late resource missed cancellation")
	}
	work.Finish()
	final, err := closing.WaitReleased(ctx)
	if err != nil || !final.Released || closed.Load() != 1 {
		t.Fatalf("owner failed to finish: %+v %v", final, err)
	}
	if _, err := lifetime.Enter(ctx); !errors.Is(err, qimu.ErrClosed) {
		t.Fatalf("work admitted after seal: %v", err)
	}
}

func TestCloseFailureHistoryIsBoundedAndErrorsDoNotNegateReleasedEvidence(t *testing.T) {
	lifetime := qimu.NewLifetime(context.Background())
	for range 80 {
		if _, err := lifetime.Own(qimu.Resource{Name: "released with diagnostic", Close: func(context.Context) (qimu.ReleaseState, error) {
			return qimu.ResourceReleased, errors.New("diagnostic")
		}}); err != nil {
			t.Fatal(err)
		}
	}
	final, err := lifetime.Close(testContext(t))
	if !final.Released || err == nil || len(final.Failures) != 32 || final.OmittedFailures != 48 {
		t.Fatalf("incorrect bounded conclusion: %+v %v", final, err)
	}
}
