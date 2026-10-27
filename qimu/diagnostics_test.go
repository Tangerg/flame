package qimu_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/qimu"
)

func TestInstanceObservationProjectsOriginalCloseEvidence(t *testing.T) {
	ctx := testContext(t)
	host, err := qimu.NewHost(context.Background(), qimu.HostConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var handle *qimu.ResourceHandle
	t.Cleanup(func() {
		if handle != nil {
			if err := handle.ConfirmReleased(nil); err != nil {
				t.Error(err)
			}
		}
		outcome, err := host.Close(testContext(t))
		if !outcome.Released {
			t.Errorf("host did not release after original proof: %+v, %v", outcome, err)
		}
	})
	service, err := qimu.NewContract[string]("diagnostics.output", "1")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Register(service); err != nil {
		t.Fatal(err)
	}
	cleanupFailure := errors.New("original connection still lacks termination evidence")
	var setup *qimu.Setup
	var cleanupCalls atomic.Int64
	plugin := installNative(t, host, "diagnostics", qimu.Unit{
		Declaration: qimu.UnitDeclaration{
			ID:      "main",
			Profile: qimu.NativeGo,
			Exports: []qimu.ExportDeclaration{{Name: "output", Contract: service.Ref()}},
		},
		Setup: func(_ context.Context, current *qimu.Setup) ([]qimu.ExportValue, error) {
			setup = current
			var err error
			handle, err = current.Own(qimu.Resource{
				Name: "original connection",
				Close: func(context.Context) (qimu.ReleaseState, error) {
					cleanupCalls.Add(1)
					return qimu.ResourceUnconfirmed, cleanupFailure
				},
			})
			if err != nil {
				return nil, err
			}
			return []qimu.ExportValue{qimu.Provide("output", service, "value")}, nil
		},
	})
	if _, err := host.Activate(ctx, source("diagnostics", plugin, "main", "output", service.Ref())); err != nil {
		t.Fatal(err)
	}
	ready := host.Observations()
	if len(ready) != 1 || ready[0].State != qimu.Ready || ready[0].Close != nil {
		t.Fatalf("ready observation invented an exit: %+v", ready)
	}
	if err := host.RetireInstance(ctx, setup.InstanceID()); err != nil {
		t.Fatal(err)
	}
	exit := setup.Lifetime().BeginClose(qimu.ErrClosed)
	original, err := exit.Wait(ctx)
	if !errors.Is(err, qimu.ErrUnconfirmedRelease) || original.Released {
		t.Fatalf("cleanup failure became release proof: %+v, %v", original, err)
	}
	observations := host.Observations()
	if len(observations) != 1 || observations[0].State != qimu.CloseFailed {
		t.Fatalf("unconfirmed instance was hidden: %+v", observations)
	}
	observed := observations[0].Close
	if observed == nil || observed.ID != original.ID || observed.Released ||
		!slices.Equal(observed.Pending, []string{"original connection"}) ||
		!slices.Equal(observed.Failures, []qimu.CloseFailure{{Resource: "original connection", Detail: cleanupFailure.Error()}}) {
		t.Fatalf("observation lost original exit evidence: %+v", observed)
	}
	observed.ID = 0
	observed.Pending[0] = "changed projection"
	observed.Failures[0].Detail = "changed projection"
	fresh := host.Observations()[0].Close
	if fresh.ID != original.ID || !slices.Equal(fresh.Pending, original.Pending) || !slices.Equal(fresh.Failures, original.Failures) {
		t.Fatalf("mutable observation changed the original exit: %+v", fresh)
	}
	if err := handle.ConfirmReleased(nil); err != nil {
		t.Fatal(err)
	}
	if err := host.WaitInstanceRetired(ctx, setup.InstanceID()); err != nil {
		t.Fatal(err)
	}
	final, err := exit.WaitReleased(ctx)
	if !final.Released || final.ID != original.ID || err == nil || !slices.Equal(final.Failures, original.Failures) {
		t.Fatalf("late proof replaced original diagnostics: %+v, %v", final, err)
	}
	if cleanupCalls.Load() != 1 || len(host.Observations()) != 0 {
		t.Fatalf("late proof replayed cleanup or retained a terminal instance: calls=%d observations=%+v", cleanupCalls.Load(), host.Observations())
	}
}

type diagnosticUseOwner struct {
	id     string
	values []qimu.Dependency
	reads  int
}

func (o *diagnosticUseOwner) ID() string { return o.id }

func (o *diagnosticUseOwner) Held(context.Context, qimu.InstallationID) ([]qimu.Dependency, error) {
	o.reads++
	return o.values, nil
}

func TestRetainedDependenciesIdentifyTheirOriginalOwners(t *testing.T) {
	admission := qimu.NewAdmission()
	dependency := qimu.Dependency{InstallationID: "original", Release: qimu.Hash([]byte("release"))}
	documents := &diagnosticUseOwner{id: "consumer.documents", values: []qimu.Dependency{dependency}}
	tasks := &diagnosticUseOwner{id: "consumer.tasks", values: []qimu.Dependency{dependency, dependency}}
	for _, owner := range []*diagnosticUseOwner{tasks, documents} {
		if err := admission.RegisterUseOwner(owner); err != nil {
			t.Fatal(err)
		}
	}
	retained, err := admission.Retained(t.Context(), "original")
	if err != nil {
		t.Fatal(err)
	}
	expected := []qimu.RetainedDependency{
		{OwnerID: documents.ID(), Dependency: dependency},
		{OwnerID: tasks.ID(), Dependency: dependency},
	}
	if !slices.Equal(retained, expected) {
		t.Fatalf("retention lost original owner identity: %+v", retained)
	}
	retained[0].Dependency.Release = qimu.Hash([]byte("changed projection"))
	retained[0].OwnerID = "changed projection"
	if documents.values[0] != dependency {
		t.Fatal("diagnostic projection mutated its business owner")
	}
	admission.RequireRecovery(errors.New("another owner needs recovery"))
	documents.values = nil
	fresh, err := admission.Retained(t.Context(), "original")
	if err != nil || !slices.Equal(fresh, expected[1:]) {
		t.Fatalf("diagnostics did not read current original records behind recovery: %+v, %v", fresh, err)
	}
	held, err := admission.Held(t.Context(), "original")
	if err != nil || !slices.Equal(held, []qimu.Dependency{dependency}) {
		t.Fatalf("destructive commands observed a different retention source: %+v, %v", held, err)
	}
	if documents.reads != 3 || tasks.reads != 3 {
		t.Fatalf("retention reads were cached or duplicated: documents=%d tasks=%d", documents.reads, tasks.reads)
	}
}

func TestScopeObservationDoesNotReopenAFencedGenerationDuringRecovery(t *testing.T) {
	ctx := testContext(t)
	admission := qimu.NewAdmission()
	const id qimu.ScopeID = "original-scope"
	if observed, exists := admission.Scope(id); exists {
		t.Fatalf("unknown scope was invented: %+v", observed)
	}
	grant, err := admission.OpenScope(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	original, exists := admission.Scope(id)
	if !exists || original.ID != id || original.Generation != grant.Generation() || original.Fenced {
		t.Fatalf("scope observation does not identify original grant: %+v", original)
	}
	admission.RequireRecovery(errors.New("restore original resource evidence"))
	entered := make(chan struct{})
	release := make(chan struct{})
	released := false
	t.Cleanup(func() {
		if !released {
			close(release)
		}
	})
	maintenance := make(chan error, 1)
	go func() {
		maintenance <- admission.Maintenance(ctx, func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	fenced := make(chan struct{})
	go func() {
		admission.FenceScope(id)
		close(fenced)
	}()
	select {
	case <-fenced:
	case <-ctx.Done():
		t.Fatal("scope fence waited for the occupied admission gate")
	}
	observed, exists := admission.Scope(id)
	if !exists || observed.Generation != original.Generation || !observed.Fenced {
		t.Fatalf("recovery hid the original local fence: %+v", observed)
	}
	if err := admission.ValidateScope(grant); !errors.Is(err, qimu.ErrFenced) {
		t.Fatalf("fenced grant remained usable: %v", err)
	}
	close(release)
	released = true
	if err := <-maintenance; err != nil {
		t.Fatal(err)
	}
	if _, err := admission.OpenScope(ctx, id); !errors.Is(err, qimu.ErrRecoveryRequired) {
		t.Fatalf("scope reopened before recovery completed: %v", err)
	}
	if err := admission.Recover(ctx, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	recovered, _ := admission.Scope(id)
	if recovered != observed || !errors.Is(admission.ValidateScope(grant), qimu.ErrFenced) {
		t.Fatalf("recovery reopened the original scope: %+v", recovered)
	}
	renewed, err := admission.OpenScope(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	current, _ := admission.Scope(id)
	if current.Fenced || current.Generation != original.Generation+1 || renewed.Generation() != current.Generation {
		t.Fatalf("explicit reopening did not create a new generation: %+v", current)
	}
	if err := admission.ValidateScope(renewed); err != nil {
		t.Fatal(err)
	}
	if err := admission.ValidateScope(grant); !errors.Is(err, qimu.ErrFenced) {
		t.Fatalf("new scope generation revived an old grant: %v", err)
	}
}
