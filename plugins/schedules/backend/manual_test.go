package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	models "github.com/Tangerg/flame/packages/go-sdk/model/catalog"
	rpc "github.com/Tangerg/flame/plugins/schedules/internal/contract"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
	"github.com/Tangerg/flame/plugins/schedules/internal/sqlite"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestPublicRunNowOwnsCaptureAndOriginalAcceptanceReceipt(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := sqlite.NewScheduleStore(db)
	definition, err := schedule.New("sch_manual", schedule.Draft{Title: "Captured", Instructions: "original instructions", Cron: "@hourly"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(ctx, definition); err != nil {
		t.Fatal(err)
	}
	host := &observedRunHost{db: db, namespace: "original-runtime"}
	host.lost.Store(true)
	server := sdk.NewServer(&sdk.Implementation{Name: "schedules-test"}, nil)
	registered, err := registerServer(server, db, store, filesystem.DirectoryResolver{}, models.Selections{}, func(context.Context, qimumcp.CallbackRef) (runHost, func() error, error) {
		return host, func() error { return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := registered.Close(); err != nil {
			t.Error(err)
		}
	})
	client := sdk.NewClient(&sdk.Implementation{Name: "public-author"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	left, right := sdk.NewInMemoryTransports()
	backend, err := server.Connect(ctx, left, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	session, err := client.Connect(ctx, right, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	exported, err := qimumcp.ResolveCapability(ctx, session, "schedules", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	method, err := exported.Method(rpc.RunNow)
	if err != nil {
		t.Fatal(err)
	}
	invocation := portable.Invocation{ID: "original-manual", Origin: extension.Human, Target: portable.Target{Scope: "installation", Reference: "captured-installation"}}
	ctx, err = qimumcp.WithInvocation(ctx, invocation)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := qimumcp.CallbackMetadata(qimumcp.CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: strings.Repeat("a", 64), Call: strings.Repeat("b", 64), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Encode(rpc.ItemInput{ID: definition.ID()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := method.InvokeWithMetadata(ctx, invocation.ID, input, metadata); err == nil {
		t.Fatal("lost acceptance was reported as success")
	}
	pending, err := store.PendingManual(ctx, "", 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("capture: %v, %v", pending, err)
	}
	captured := pending[0]
	if captured.Invocation() != invocation || captured.Snapshot().Execution.Instructions != definition.Instructions() {
		t.Fatal("manual capture lost its original caller or input")
	}
	if _, found, err := method.ReadReceipt(ctx, invocation.ID, input); err != nil || found {
		t.Fatalf("unknown receipt: %t, %v", found, err)
	}
	if _, err := store.Delete(ctx, definition.ID()); err != nil {
		t.Fatal(err)
	}
	host.lost.Store(false)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER refuse_manual_reply BEFORE UPDATE OF reply ON commands BEGIN SELECT RAISE(ABORT,'receipt failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := method.InvokeWithMetadata(ctx, invocation.ID, input, metadata); err == nil {
		t.Fatal("failed private receipt commit became success")
	}
	pending, err = store.PendingManual(ctx, "", 10)
	if err != nil || len(pending) != 1 || pending[0].Snapshot() != captured.Snapshot() {
		t.Fatalf("receipt failure changed original pending work: %v", err)
	}
	if _, found, err := method.ReadReceipt(ctx, invocation.ID, input); err != nil || found {
		t.Fatalf("failed receipt fabricated completion: %t, %v", found, err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER refuse_manual_reply`); err != nil {
		t.Fatal(err)
	}
	recovered, err := resumeCaptured(ctx, db, store, host, captured)
	if err != nil || recovered.RunID != captured.RunID() || recovered.SessionID != captured.SessionID() {
		t.Fatalf("background recovery changed original result: %+v, %v", recovered, err)
	}
	callsAfterRecovery := host.calls.Load()

	value, err := method.InvokeWithMetadata(ctx, invocation.ID, input, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if host.calls.Load() != callsAfterRecovery {
		t.Fatal("public replay repeated a recovered Run admission")
	}
	calls := host.calls.Load()
	if receipt, found, err := method.ReadReceipt(ctx, invocation.ID, input); err != nil || !found || receipt != value {
		t.Fatalf("original receipt: %t, %v", found, err)
	}
	if replay, err := method.Invoke(ctx, invocation.ID, input); err != nil || replay != value || host.calls.Load() != calls {
		t.Fatalf("completed replay reacquired collaborators: %v", err)
	}
	pending, err = store.PendingManual(ctx, "", 10)
	if err != nil || len(pending) != 0 {
		t.Fatalf("accepted work remained pending: %v, %v", pending, err)
	}
	retained, err := store.CapturedRun(ctx, captured.RunID())
	if err != nil || retained.Snapshot() != captured.Snapshot() {
		t.Fatalf("deleted definition changed captured work: %v", err)
	}
	changed, err := jsonvalue.Encode(rpc.ItemInput{ID: "sch_other"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := method.Invoke(ctx, invocation.ID, changed); !errors.Is(decodeError(err), portable.ErrInvocationConflict) {
		t.Fatalf("changed input did not conflict: %v", err)
	}
	refused := invocation
	refused.ID = "missing-schedule"
	refusalContext, err := qimumcp.WithInvocation(t.Context(), refused)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := method.InvokeWithMetadata(refusalContext, refused.ID, changed, metadata); !errors.Is(decodeError(err), schedule.ErrNotFound) {
		t.Fatalf("business refusal: %v", err)
	}
	if _, found, err := method.ReadReceipt(refusalContext, refused.ID, changed); !found || !errors.Is(decodeError(err), schedule.ErrNotFound) {
		t.Fatalf("business refusal was not retained: %t, %v", found, err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("multiple private outcome owners: %d, %v", rows, err)
	}
}
