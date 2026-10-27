package qimu_test

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"weak"

	"github.com/Tangerg/flame/qimu"
)

type retainedPayload struct{ bytes [1 << 20]byte }

func awaitCollected[T any](t *testing.T, value weak.Pointer[T]) {
	t.Helper()
	for range 100 {
		runtime.GC()
		if value.Value() == nil {
			return
		}
		runtime.Gosched()
	}
	t.Fatal("terminal handle retained its former resource owner")
}

func closedLifetime(t *testing.T) (*qimu.Close, weak.Pointer[retainedPayload]) {
	t.Helper()
	value := new(retainedPayload)
	value.bytes[0] = 1
	type key struct{}
	owner := qimu.NewLifetime(context.WithValue(context.Background(), key{}, value))
	exit := owner.BeginClose(qimu.ErrClosed)
	if outcome, err := exit.WaitReleased(t.Context()); err != nil || !outcome.Released {
		t.Fatalf("close: %+v, %v", outcome, err)
	}
	return exit, weak.Make(value)
}

func TestTerminalExitDoesNotRetainOriginalContext(t *testing.T) {
	exit, value := closedLifetime(t)
	awaitCollected(t, value)
	if outcome, err := exit.Wait(t.Context()); err != nil || !outcome.Released {
		t.Fatalf("retained observation: %+v, %v", outcome, err)
	}
	runtime.KeepAlive(exit)
}

func closedUse(t *testing.T) (*qimu.Use, weak.Pointer[qimu.Host], weak.Pointer[retainedPayload]) {
	t.Helper()
	ctx := t.Context()
	host, err := qimu.NewHost(ctx, qimu.HostConfig{})
	if err != nil {
		t.Fatal(err)
	}
	service, err := qimu.NewContract[*retainedPayload]("retention.value", "1")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Register(service); err != nil {
		t.Fatal(err)
	}
	value := new(retainedPayload)
	value.bytes[0] = 1
	plugin, err := qimu.Define(qimu.Definition{Revision: qimu.Hash([]byte("retention")), Units: []qimu.Unit{{
		Declaration: qimu.UnitDeclaration{ID: "value", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "value", Contract: service.Ref()}}},
		Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{qimu.Provide("value", service, value)}, nil
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Install(ctx, qimu.InstallationConfig{ID: "retention", Plugin: plugin, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	source := qimu.ExportRef{InstallationID: "retention", Release: plugin.Revision(), UnitID: "value", Export: "value", Contract: service.Ref()}
	binding, err := host.Activate(ctx, source)
	if err != nil {
		t.Fatal(err)
	}
	owner := qimu.NewLifetime(ctx)
	scope, err := host.Admission().OpenScope(ctx, "retention")
	if err != nil {
		t.Fatal(err)
	}
	use, err := host.AcquireBinding(ctx, binding, qimu.UseConfig{Owner: owner, Scope: scope})
	if err != nil {
		t.Fatal(err)
	}
	if err := qimu.BorrowExport(ctx, use, source, func(_ context.Context, actual *retainedPayload) error {
		if actual != value {
			t.Fatal("borrow did not preserve the original native object")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if outcome, err := use.Close(ctx); err != nil || !outcome.Released {
		t.Fatalf("use close: %+v, %v", outcome, err)
	}
	if _, err := owner.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if outcome, err := host.Close(ctx); err != nil || !outcome.Released {
		t.Fatalf("host close: %+v, %v", outcome, err)
	}
	return use, weak.Make(host), weak.Make(value)
}

func TestTerminalUseDoesNotRetainHostOrNativeObject(t *testing.T) {
	use, host, value := closedUse(t)
	awaitCollected(t, host)
	awaitCollected(t, value)
	if outcome, err := use.Close(t.Context()); err != nil || !outcome.Released {
		t.Fatalf("original exit: %+v, %v", outcome, err)
	}
	if use.ID() == "" || len(use.Binding().Roots) != 1 {
		t.Fatal("terminal use lost its immutable identity")
	}
	if err := qimu.BorrowExport(t.Context(), use, use.Sources()[0], func(context.Context, *retainedPayload) error {
		t.Fatal("terminal use dispatched work")
		return nil
	}); !errors.Is(err, qimu.ErrClosed) {
		t.Fatalf("terminal borrow: %v", err)
	}
	runtime.KeepAlive(use)
}
