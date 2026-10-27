package mcp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestParsedMutationResultSurvivesLaterCallerCancellationAndUseExit(t *testing.T) {
	method := testMethod[string, string](t, "records.write", portable.Mutation)
	var calls atomic.Int64
	session := methodSession(t, func(backend *Backend) error {
		return method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) (string, error) {
			calls.Add(1)
			if command.ID != "original-operation" || command.Input != "original-input" {
				return "", errors.New("original invocation changed")
			}
			return "committed-result", nil
		})
	}, method.Name())
	host, err := qimu.NewHost(t.Context(), qimu.HostConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if outcome, err := host.Close(context.Background()); err != nil || !outcome.Released {
			t.Errorf("Host close: %+v, %v", outcome, err)
		}
	})
	contract, err := qimu.NewContract[func(context.Context) (string, error)]("consumer.accepted-result", "1")
	if err != nil {
		t.Fatal(err)
	}
	if err := host.Register(contract); err != nil {
		t.Fatal(err)
	}
	caller, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	cause := errors.New("caller stopped after receiving the original response")
	parsed := false
	plugin, err := qimu.Define(qimu.Definition{Revision: qimu.Hash([]byte("known-result-consumer")), Units: []qimu.Unit{{
		Declaration: qimu.UnitDeclaration{ID: "main", Profile: qimu.NativeGo, Exports: []qimu.ExportDeclaration{{Name: "invoke", Contract: contract.Ref()}}},
		Setup: func(context.Context, *qimu.Setup) ([]qimu.ExportValue, error) {
			return []qimu.ExportValue{qimu.Provide("invoke", contract, func(ctx context.Context) (string, error) {
				value, err := method.CallCommand(ctx, session, "original-operation", "original-input")
				if err == nil {
					parsed = true
					cancel(cause)
				}
				return value, err
			})}, nil
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Install(t.Context(), qimu.InstallationConfig{ID: "consumer", Plugin: plugin, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	root := qimu.ExportRef{InstallationID: "consumer", Release: plugin.Revision(), UnitID: "main", Export: "invoke", Contract: contract.Ref()}
	plan, err := host.Plan(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := host.Admission().OpenScope(t.Context(), "original-caller")
	if err != nil {
		t.Fatal(err)
	}
	var result string
	var closes atomic.Int64
	err = qimu.WithPlan(caller, host, plan, scope, func(use *qimu.Use) error {
		if _, err := use.Lifetime().Cleanup("original call resource", func() error { closes.Add(1); return nil }); err != nil {
			return err
		}
		return qimu.BorrowExport(caller, use, root, func(ctx context.Context, invoke func(context.Context) (string, error)) error {
			var err error
			result, err = invoke(ctx)
			return err
		})
	})
	if err != nil || result != "committed-result" || !parsed || calls.Load() != 1 || closes.Load() != 1 {
		t.Fatalf("known result was replaced or replayed during cancellation: result=%q parsed=%v calls=%d closes=%d error=%v", result, parsed, calls.Load(), closes.Load(), err)
	}
	if !errors.Is(caller.Err(), context.Canceled) || !errors.Is(context.Cause(caller), cause) {
		t.Fatalf("keeping the result erased the independent cancellation cause: %v, %v", caller.Err(), context.Cause(caller))
	}
}
