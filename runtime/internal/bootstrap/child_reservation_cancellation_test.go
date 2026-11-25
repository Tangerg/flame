package bootstrap

import (
	"context"
	"database/sql/driver"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

func TestRootCancellationWhileAChildReservesItsStart(t *testing.T) {
	if !runCancellationFixture(t) {
		return
	}

	reserving, release := make(chan struct{}), make(chan struct{})
	releaseReservation := sync.OnceFunc(func() { close(release) })
	gate := sync.OnceFunc(func() {
		close(reserving)
		<-release
	})
	var reservationWritten atomic.Bool
	if err := modernsqlite.RegisterScalarFunction("test_note_child_reservation", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			reservationWritten.Store(true)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := delegateRestartModel{chat.ModelFunc(cancelTreeScript{
		childEntered: make(chan string, 2), confirmed: true,
	}.Call)}
	stores, err := persistence.Open(t.Context(), persistence.Config{
		DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	transact := stores.Transactor
	stores.Transactor = func(ctx context.Context, write func(context.Context) error) error {
		var reserved bool
		err := transact(ctx, func(ctx context.Context) error {
			err := write(ctx)
			reserved = reservationWritten.Swap(false)
			return err
		})
		// Hold the reservation's reply after SQLite releases its only connection.
		if reserved && err == nil {
			gate()
		}
		return err
	}
	host, api := buildProtocolRuntime(t, protocolRuntimeConfig(t, stores, model), home)
	defer func() {
		releaseReservation()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	questionCrashDatabase(t, home, `CREATE TRIGGER gate_child_reservation AFTER INSERT ON child_run_start_reservations
		BEGIN SELECT test_note_child_reservation(); END`)
	ctx := delivery.WithRequestMeta(t.Context(), waitingChildCancellationMeta())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "cancel the root during child reservation",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID,
		Input:     []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate both children"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	collected := collectRunEvents(events)
	waitForSignal(t, reserving, "the committed reservation before the child is admitted")

	const reason = "stop the tree before a child starts"
	if _, err := api.CancelRun(ctx, protocol.CancelRunRequest{RunID: started.RunID, Reason: reason}); !errors.Is(err, protocol.ErrInternalError) {
		t.Fatalf("cancellation should reach its settlement deadline while the reservation reply is withheld: %v", err)
	}
	waitForRunOutcomeAfter(t, ctx, api, started.RunID, protocol.OutcomeCanceled, releaseReservation)
	waitForRunEvents(t, collected, "the canceled root stream")
	root, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if root.Outcome == nil || root.Outcome.Detail != reason {
		t.Fatalf("the root did not settle the original cancellation: %+v", root.Outcome)
	}
}
