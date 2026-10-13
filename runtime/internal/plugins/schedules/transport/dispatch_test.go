package transport

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/modelcatalog"
	workspace "github.com/Tangerg/flame/runtime/internal/adapter/workspace"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type observedRunHost struct {
	db        *sql.DB
	namespace string
	calls     atomic.Int32
	lost      atomic.Bool
}

func (h *observedRunHost) Namespace(context.Context) (string, error) { return h.namespace, nil }
func (h *observedRunHost) Admit(ctx context.Context, _ schedule.RunRequest) error {
	// This read needs the database's single connection. Admission under an outer
	// private transaction would block it until this test's context expires.
	var rows int
	if err := h.db.QueryRowContext(ctx, `SELECT count(*) FROM manual_firings`).Scan(&rows); err != nil {
		return err
	}
	h.calls.Add(1)
	if h.lost.Load() {
		return errors.New("Run acceptance observation lost")
	}
	return nil
}

func TestDispatchAcknowledgementAndReceiptCommitTogether(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := sqlite.NewScheduleStore(db)
	now := time.Now().UTC()
	definition, err := schedule.New("sch_atomic", schedule.Draft{Title: "Captured", Instructions: "inspect", Cron: "@hourly"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Insert(ctx, definition); err != nil {
		t.Fatal(err)
	}
	request, err := definition.ManualRun("ses_atomic", "run_atomic", now.Add(time.Second), testsupport.ScheduleInvocation())
	if err != nil {
		t.Fatal(err)
	}
	ctx, err = pluginsdk.WithInvocation(ctx, request.Invocation())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.QueueManual(ctx, request); err != nil {
		t.Fatal(err)
	}
	host := &observedRunHost{db: db, namespace: "original-store"}
	server := sdk.NewServer(&sdk.Implementation{Name: "schedules-test"}, nil)
	registered, err := RegisterServer(server, db, store, workspace.Resolver{}, models.Selections{}, func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error) {
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
	client := sdk.NewClient(&sdk.Implementation{Name: "host-test"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	left, right := sdk.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, left, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := client.Connect(ctx, right, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	exported, err := pluginsdk.ResolveCapability(ctx, session, "schedules", schedulesContract(t))
	if err != nil {
		t.Fatal(err)
	}
	method, err := exported.Method(rpc.Dispatch)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := pluginsdk.CallbackMetadata(pluginsdk.CallbackRef{Endpoint: "http://127.0.0.1:1/", Credential: strings.Repeat("a", 64), Call: strings.Repeat("b", 64), MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsoninput.Encode(rpc.DispatchInput{RunID: request.RunID()})
	if err != nil {
		t.Fatal(err)
	}
	host.lost.Store(true)
	if _, err := method.InvokeWithMetadata(ctx, "original-dispatch", input, metadata); err == nil {
		t.Fatal("lost Run acceptance became a private success")
	}
	assertPending := func() {
		t.Helper()
		var state string
		var reply sql.NullString
		if err := db.QueryRowContext(ctx, `SELECT state FROM manual_firings WHERE run_id=?`, request.RunID()).Scan(&state); err != nil || state != "pending" {
			t.Fatalf("pending state=%s error=%v", state, err)
		}
		if err := db.QueryRowContext(ctx, `SELECT reply FROM commands WHERE id='original-dispatch'`).Scan(&reply); err != nil || reply.Valid {
			t.Fatalf("unconfirmed reply=%+v error=%v", reply, err)
		}
	}
	assertPending()
	host.lost.Store(false)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER refuse_reply BEFORE UPDATE OF reply ON commands BEGIN SELECT RAISE(ABORT,'receipt failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := method.InvokeWithMetadata(ctx, "original-dispatch", input, metadata); err == nil {
		t.Fatal("failed receipt commit became known")
	}
	assertPending()
	if current, err := store.Get(ctx, definition.ID()); err != nil || current.Revision() != definition.Revision() {
		t.Fatalf("definition advanced outside receipt transaction: %+v, %v", current.Snapshot(), err)
	}
	if _, err := db.ExecContext(ctx, `DROP TRIGGER refuse_reply`); err != nil {
		t.Fatal(err)
	}
	result, err := method.InvokeWithMetadata(ctx, "original-dispatch", input, metadata)
	if err != nil {
		t.Fatal(err)
	}
	var reference app.RunReference
	if err := json.Unmarshal(result.JSON(), &reference); err != nil || reference.SessionID != request.SessionID() || reference.RunID != request.RunID() {
		t.Fatalf("accepted reference=%+v error=%v", reference, err)
	}
	calls := host.calls.Load()
	if replay, err := method.InvokeWithMetadata(ctx, "original-dispatch", input, metadata); err != nil || replay != result || host.calls.Load() != calls {
		t.Fatalf("known receipt repeated Run admission: %v, calls=%d", err, host.calls.Load())
	}
	changed, err := jsoninput.Encode(rpc.DispatchInput{RunID: "run_changed"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = method.InvokeWithMetadata(ctx, "original-dispatch", changed, metadata)
	failure, known := errors.AsType[*capability.Refusal](err)
	if !known || failure.Code != "command_conflict" || host.calls.Load() != calls {
		t.Fatalf("changed input entered admission: %v", err)
	}
}

func TestCapturedRuntimeTargetSurvivesRestartAndRefusesAnotherStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "schedules.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := bindRuntime(t.Context(), db, "original"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := bindRuntime(t.Context(), db, "original"); err != nil {
		t.Fatal(err)
	}
	if err := bindRuntime(t.Context(), db, "another"); err == nil {
		t.Fatal("captured target was replaced")
	}
	var original string
	if err := db.QueryRow(`SELECT namespace FROM runtime_target WHERE id=1`).Scan(&original); err != nil || original != "original" {
		t.Fatalf("original target=%q error=%v", original, err)
	}
}

func schedulesContract(t *testing.T) *capability.Contract {
	t.Helper()
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	return contracts.Capability
}
