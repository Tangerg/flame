package qimu_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/qimu"
)

func TestLifetimeWorkCapacityDoesNotCancelAcceptedWork(t *testing.T) {
	ctx := testContext(t)
	lifetime, err := qimu.NewLifetimeWithLimits(context.Background(), qimu.LifetimeLimits{MaxWork: 2, MaxResources: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := lifetime.Close(testContext(t)); err != nil {
			t.Error(err)
		}
	})
	type result struct {
		work *qimu.Work
		err  error
	}
	completed := make(chan result, 16)
	for range 16 {
		go func() {
			work, err := lifetime.Enter(ctx)
			completed <- result{work, err}
		}()
	}
	var accepted []*qimu.Work
	for range 16 {
		result := <-completed
		if result.err == nil {
			accepted = append(accepted, result.work)
		} else if !errors.Is(result.err, qimu.ErrCapacity) {
			t.Fatalf("unexpected work refusal: %v", result.err)
		}
	}
	if len(accepted) != 2 {
		t.Fatalf("concurrent admission exceeded work budget: %d", len(accepted))
	}
	for _, work := range accepted {
		if err := work.Context().Err(); err != nil {
			t.Fatalf("capacity refusal canceled accepted work: %v", err)
		}
	}
	accepted[0].Finish()
	replacement, err := lifetime.Enter(ctx)
	if err != nil {
		t.Fatalf("original completion did not release work capacity: %v", err)
	}
	replacement.Finish()
	accepted[1].Finish()
}

func TestLifetimeResourceCapacityWaitsForOriginalProof(t *testing.T) {
	ctx := testContext(t)
	lifetime, err := qimu.NewLifetimeWithLimits(context.Background(), qimu.LifetimeLimits{MaxWork: 1, MaxResources: 1})
	if err != nil {
		t.Fatal(err)
	}
	var originalCloses, rejectedCloses atomic.Int64
	original, err := lifetime.Own(qimu.Resource{Name: "original", Close: func(context.Context) (qimu.ReleaseState, error) {
		originalCloses.Add(1)
		return qimu.ResourceUnconfirmed, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	rejected := qimu.Resource{Name: "new", Close: func(context.Context) (qimu.ReleaseState, error) {
		rejectedCloses.Add(1)
		return qimu.ResourceReleased, nil
	}}
	if _, err := lifetime.Own(rejected); !errors.Is(err, qimu.ErrCapacity) {
		t.Fatalf("resource capacity was not enforced: %v", err)
	}
	work, err := lifetime.Enter(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := work.Own(rejected); !errors.Is(err, qimu.ErrCapacity) {
		t.Fatalf("work ownership bypassed the resource budget: %v", err)
	}
	if originalCloses.Load() != 0 || rejectedCloses.Load() != 0 {
		t.Fatal("capacity admission evicted or closed a resource")
	}
	if err := original.ConfirmReleased(nil); err != nil {
		t.Fatal(err)
	}
	pending, err := lifetime.Own(qimu.Resource{Name: "pending", Close: func(context.Context) (qimu.ReleaseState, error) {
		originalCloses.Add(1)
		return qimu.ResourceUnconfirmed, nil
	}})
	if err != nil {
		t.Fatalf("original proof did not release capacity: %v", err)
	}
	closing := lifetime.BeginClose(qimu.ErrClosed)
	if _, err := work.Own(rejected); !errors.Is(err, qimu.ErrCapacity) {
		t.Fatalf("closing work dropped an unconfirmed resource to admit another: %v", err)
	}
	work.Finish()
	first, err := closing.Wait(ctx)
	if !errors.Is(err, qimu.ErrUnconfirmedRelease) || first.Released || len(first.Pending) != 1 || first.Pending[0] != "pending" || originalCloses.Load() != 1 {
		t.Fatalf("original unknown resource was lost: %+v, %v; closes=%d", first, err, originalCloses.Load())
	}
	if err := pending.ConfirmReleased(nil); err != nil {
		t.Fatal(err)
	}
	final, err := closing.WaitReleased(ctx)
	if err != nil || !final.Released || final.ID != first.ID || originalCloses.Load() != 1 {
		t.Fatalf("proof did not finish the original retirement: %+v, %v", final, err)
	}
}

func TestHostUseCapacityIncludesUnconfirmedRetirement(t *testing.T) {
	ctx := testContext(t)
	host := testHost(t, qimu.HostConfig{MaxUses: 1})
	point, _ := qimu.NewPoint[string, string]("example.capacity", "1")
	if err := host.Register(point); err != nil {
		t.Fatal(err)
	}
	plugin := installNative(t, host, "provider", qimu.Unit{
		Declaration: qimu.UnitDeclaration{ID: "main", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "value", Contract: point.Ref()}}},
		Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{qimu.Contribute("value", point, "metadata", "original")}, nil
		},
	})
	if _, err := host.Activate(ctx, source("provider", plugin, "main", "value", point.Ref())); err != nil {
		t.Fatal(err)
	}
	snapshot, err := host.Snapshot(ctx, point.Ref())
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := qimu.Candidates(snapshot, point)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := qimu.Select(snapshot, point, "original@1", candidates[0])
	if err != nil {
		t.Fatal(err)
	}
	plan, err := qimu.Capture(snapshot, selection.Ref())
	if err != nil {
		t.Fatal(err)
	}
	owner, scope := ownerAndScope(t, host)
	config := qimu.UseConfig{Owner: owner, Scope: scope}
	use, err := host.Acquire(ctx, plan, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Acquire(ctx, plan, config); !errors.Is(err, qimu.ErrCapacity) {
		t.Fatalf("new Use exceeded the Host budget: %v", err)
	}
	if err := qimu.Borrow(ctx, use, selection, func(_ context.Context, values []string) error {
		if len(values) != 1 || values[0] != "original" {
			t.Fatal("capacity refusal changed the original Use")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	var closes atomic.Int64
	resource, err := use.Lifetime().Own(qimu.Resource{Name: "browser object", Close: func(context.Context) (qimu.ReleaseState, error) {
		closes.Add(1)
		return qimu.ResourceUnconfirmed, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := use.Close(ctx)
	if !errors.Is(err, qimu.ErrUnconfirmedRelease) || outcome.Released || closes.Load() != 1 {
		t.Fatalf("uncertain Use retirement became final: %+v, %v", outcome, err)
	}
	if _, err := host.Acquire(ctx, plan, config); !errors.Is(err, qimu.ErrCapacity) {
		t.Fatalf("unknown Use released its capacity: %v", err)
	}
	if err := resource.ConfirmReleased(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := use.BeginClose(qimu.ErrClosed).WaitReleased(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := use.Close(ctx); err != nil {
		t.Fatal(err)
	}
	next, err := host.Acquire(ctx, plan, config)
	if err != nil {
		t.Fatalf("settled original Use did not release capacity: %v", err)
	}
	if next.ID() == use.ID() || closes.Load() != 1 {
		t.Fatal("new admission reused or restarted the retired Use")
	}
}

func TestWatchCapacityRefusesNewStreamsWithoutEvictingOriginal(t *testing.T) {
	ctx := testContext(t)
	host := testHost(t, qimu.HostConfig{MaxWatches: 1, WatchCapacity: 1})
	point, _ := qimu.NewPoint[string, string]("example.watched", "1")
	if err := host.Register(point); err != nil {
		t.Fatal(err)
	}
	_, original, err := host.Watch(ctx, point.Ref())
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	if snapshot, watch, err := host.Watch(ctx, point.Ref()); !errors.Is(err, qimu.ErrCapacity) || snapshot != nil || watch != nil {
		t.Fatalf("watch capacity admitted a partial stream: %v, %v, %v", snapshot, watch, err)
	}
	plugin := installNative(t, host, "watched", qimu.Unit{
		Declaration: qimu.UnitDeclaration{ID: "main", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "value", Contract: point.Ref()}}},
		Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{qimu.Contribute("value", point, "metadata", "value")}, nil
		},
	})
	if _, err := host.Activate(ctx, source("watched", plugin, "main", "value", point.Ref())); err != nil {
		t.Fatal(err)
	}
	select {
	case change, open := <-original.Changes():
		if !open || change.Err != nil || change.Contract != point.Ref() || change.Revision == 0 {
			t.Fatalf("capacity refusal evicted the original watch: %+v, open=%v", change, open)
		}
	case <-ctx.Done():
		t.Fatal("original watch stopped observing")
	}
	original.Close()
	_, next, err := host.Watch(ctx, point.Ref())
	if err != nil {
		t.Fatalf("watch closure did not release capacity: %v", err)
	}
	next.Close()
}

func TestNegativeCoreBudgetsAreRejected(t *testing.T) {
	for _, limits := range []qimu.LifetimeLimits{{MaxWork: -1}, {MaxResources: -1}} {
		if lifetime, err := qimu.NewLifetimeWithLimits(t.Context(), limits); !errors.Is(err, qimu.ErrInvalid) || lifetime != nil {
			t.Fatalf("invalid lifetime budget was accepted: %v, %v", lifetime, err)
		}
	}
	for _, config := range []qimu.HostConfig{{MaxUses: -1}, {MaxWatches: -1}} {
		if host, err := qimu.NewHost(t.Context(), config); !errors.Is(err, qimu.ErrInvalid) || host != nil {
			t.Fatalf("invalid Host budget was accepted: %v, %v", host, err)
		}
	}
}

func TestHostResourceCapacityRefusalClosesItsOriginalUnpublishedUse(t *testing.T) {
	ctx := testContext(t)
	hostLifetime, err := qimu.NewLifetimeWithLimits(context.Background(), qimu.LifetimeLimits{MaxResources: 1})
	if err != nil {
		t.Fatal(err)
	}
	host := testHost(t, qimu.HostConfig{Lifetime: hostLifetime})
	contract, _ := qimu.NewContract[string]("example.host-resource-budget", "1")
	if err := host.Register(contract); err != nil {
		t.Fatal(err)
	}
	root := optionalExport(t, host, contract, "provider")
	binding, err := host.Activate(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	owner := qimu.NewLifetime(context.Background())
	scope, err := host.Admission().OpenScope(ctx, "capacity-owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.AcquireBinding(ctx, binding, qimu.UseConfig{Owner: owner, Scope: scope}); !errors.Is(err, qimu.ErrCapacity) {
		t.Fatalf("Host resource budget did not reject the new Use: %v", err)
	}
	outcome, err := owner.BeginClose(qimu.ErrClosed).WaitReleased(ctx)
	if err != nil || !outcome.Released || len(outcome.Failures) != 0 {
		t.Fatalf("admission refusal became a fabricated cleanup failure: %+v, %v", outcome, err)
	}
}
