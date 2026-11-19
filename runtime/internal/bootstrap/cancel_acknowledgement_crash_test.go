package bootstrap

import (
	"bytes"
	"context"
	"database/sql/driver"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	modernsqlite "modernc.org/sqlite"
)

const (
	cancelTerminalWrite  = "terminal write"
	cancelTerminalCommit = "terminal commit"
	cancelReceiptWrite   = "receipt write"
	cancelReceiptCommit  = "receipt commit"
	cancelReceiptKey     = "parked-root-original-cancellation"
	cancelReceiptReason  = "Cancel the original parked Run"
	cancelCrashCutEnv    = "FLAME_TEST_CANCEL_ACK_CUT"
	cancelCrashMarkerEnv = "FLAME_TEST_CANCEL_ACK_MARKER"
)

func TestParkedCancellationAcknowledgementCrashHelper(t *testing.T) {
	cut := os.Getenv(cancelCrashCutEnv)
	if cut == "" {
		return
	}
	if cut == cancelTerminalWrite || cut == cancelReceiptWrite {
		if err := modernsqlite.RegisterScalarFunction("test_crash_cancel_ack", 0, func(_ *modernsqlite.FunctionContext, _ []driver.Value) (driver.Value, error) {
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	home := os.Getenv("FLAME_HOME")
	ctx := protocolLifecycleContext(t.Context())
	model := newLifecycleModel()
	host, api, stores := openProtocolRuntimeWithStores(t, model)
	defer host.Close()
	fixture := &protocolLifecycleFixture{t: t, home: home, ctx: ctx, model: model, host: host, api: api, stores: stores}
	fixture.startAndPark(ctx)
	marker, err := json.Marshal(parkedProtocolMarker{SessionID: fixture.sessionID, Started: fixture.started})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv(cancelCrashMarkerEnv), marker, 0o600); err != nil {
		t.Fatal(err)
	}

	var crashStore operation.ReplayStore
	switch cut {
	case cancelTerminalWrite:
		questionCrashDatabase(t, home, `CREATE TRIGGER cancel_ack_crash BEFORE UPDATE OF payload ON idempotency_records
			WHEN NEW.kind = OLD.kind BEGIN SELECT test_crash_cancel_ack(); END`)
	case cancelTerminalCommit:
		crashStore = atomicReceiptCommitCrashStore{ReplayStore: stores.Idempotency}
	case cancelReceiptWrite:
		questionCrashDatabase(t, home, `CREATE TRIGGER cancel_ack_crash BEFORE UPDATE OF payload ON idempotency_records
			WHEN NEW.kind <> OLD.kind BEGIN SELECT test_crash_cancel_ack(); END`)
	case cancelReceiptCommit:
		crashStore = commandCrashStore{ReplayStore: stores.Idempotency, phase: commandCrashReceipt}
	default:
		t.Fatalf("unknown parked cancellation cut %q", cut)
	}
	endpoint := host.Endpoint()
	if crashStore != nil {
		endpoint, err = delivery.NewEndpoint(api, delivery.EndpointConfig{
			Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
			IdempotencyStore: crashStore, ValidateStorage: stores.ValidateDirectory,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	result := endpoint.Invoke(ctx, delivery.RunsCancel, protocol.CancelRunRequest{
		RunID: fixture.started.RunID, Reason: cancelReceiptReason,
	}, protocol.CallMetadata{IdempotencyKey: cancelReceiptKey})
	t.Fatalf("cancellation did not reach the %s cut: %+v", cut, result.Failure)
}

func TestParkedCancellationAcknowledgementSurvivesProcessCrash(t *testing.T) {
	for _, cut := range []string{cancelTerminalWrite, cancelTerminalCommit, cancelReceiptWrite, cancelReceiptCommit} {
		t.Run(cut, func(t *testing.T) { testParkedCancellationAcknowledgementCrash(t, cut) })
	}
}

func testParkedCancellationAcknowledgementCrash(t *testing.T, cut string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	path := filepath.Join(t.TempDir(), "parked-cancellation.json")
	crashCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestParkedCancellationAcknowledgementCrashHelper$")
	child.Env = append(os.Environ(), cancelCrashCutEnv+"="+cut, cancelCrashMarkerEnv+"="+path)
	out, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("cancellation did not reach the %s cut: %v\n%s", cut, err, out)
	}
	if cut == cancelTerminalWrite || cut == cancelReceiptWrite {
		questionCrashDatabase(t, home, "DROP TRIGGER cancel_ack_crash")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parked parkedProtocolMarker
	if err := json.Unmarshal(data, &parked); err != nil {
		t.Fatal(err)
	}

	model := newLifecycleModel()
	model.calls.Store(1)
	host, api := openProtocolRuntime(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	request := protocol.CancelRunRequest{RunID: parked.Started.RunID, Reason: cancelReceiptReason}
	meta := protocol.CallMetadata{IdempotencyKey: cancelReceiptKey}
	endpoint := host.Endpoint()
	current, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: request.RunID})
	if err != nil {
		t.Fatal(err)
	}
	if cut == cancelTerminalWrite {
		waiting, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: request.RunID})
		if err != nil || current.Status != protocol.RunStatusWaiting || len(waiting.Data) != 1 {
			t.Fatalf("uncommitted cancellation changed the parked Run or open question: %+v, %+v, %v", current, waiting, err)
		}
		call := endpoint.Invoke(ctx, delivery.RunsCancel, request, meta)
		if call.Failure != nil {
			t.Fatalf("same key could not cancel after rollback: %+v", call.Failure)
		}
		current, err = api.GetRun(ctx, protocol.GetRunRequest{RunID: request.RunID})
		if err != nil {
			t.Fatal(err)
		}
	}
	if current.Status != protocol.RunStatusFinished || current.Outcome == nil ||
		current.Outcome.Type != protocol.OutcomeCanceled || current.Outcome.Detail != cancelReceiptReason {
		t.Fatalf("history did not contain the exact canceled Run: %+v", current)
	}
	waiting, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: request.RunID})
	if err != nil || len(waiting.Data) != 0 {
		t.Fatalf("terminal Run kept its open question: %+v, %v", waiting, err)
	}

	if err := api.DeleteSession(ctx, parked.SessionID); err != nil {
		t.Fatalf("retire the original Run before receipt recovery: %v", err)
	}
	want := cancellationResponseJSON(t, &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: *current})
	for range 2 {
		reply, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, meta)
		if err != nil {
			t.Fatalf("recover the original cancellation: %v", err)
		}
		if got := cancellationResponseJSON(t, reply); !bytes.Equal(got, want) {
			t.Fatalf("same key lost its original terminal result after retirement:\ngot  %s\nwant %s", got, want)
		}
	}
	request.Reason = "different cancellation"
	if changed := endpoint.Invoke(ctx, delivery.RunsCancel, request, meta); !errors.Is(changed.Failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("changed cancellation reused the original key: %+v", changed)
	}
}

func cancellationResponseJSON(t *testing.T, reply *protocol.CancelRunResponse) []byte {
	t.Helper()
	data, err := json.Marshal(reply, json.Deterministic(true))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParkedCancellationAcknowledgementSurvivesSQLiteRefusal(t *testing.T) {
	for _, cut := range []string{cancelTerminalWrite, cancelReceiptWrite} {
		t.Run(cut, func(t *testing.T) {
			fixture := newProtocolLifecycleFixture(t)
			t.Cleanup(fixture.closeFirstRuntime)
			fixture.startAndPark(fixture.ctx)
			if cut == cancelTerminalWrite {
				questionCrashDatabase(t, fixture.home, `CREATE TRIGGER cancel_ack_refusal BEFORE UPDATE OF payload ON idempotency_records
					WHEN NEW.kind = OLD.kind BEGIN SELECT RAISE(ABORT, 'terminal write refused'); END`)
			} else {
				questionCrashDatabase(t, fixture.home, `CREATE TRIGGER cancel_ack_refusal BEFORE UPDATE OF payload ON idempotency_records
					WHEN NEW.kind <> OLD.kind BEGIN SELECT RAISE(ABORT, 'receipt write refused'); END`)
			}
			request := protocol.CancelRunRequest{RunID: fixture.started.RunID, Reason: cancelReceiptReason}
			meta := protocol.CallMetadata{IdempotencyKey: cancelReceiptKey}
			endpoint := fixture.host.Endpoint()
			wantProblem, wantStatus := protocol.ErrInternalError, protocol.RunStatusWaiting
			if cut == cancelReceiptWrite {
				wantProblem, wantStatus = protocol.ErrIdempotencyInProgress, protocol.RunStatusFinished
			}
			if reply := endpoint.Invoke(fixture.ctx, delivery.RunsCancel, request, meta); !errors.Is(reply.Failure, wantProblem) {
				t.Fatalf("SQLite refusal returned the wrong outcome: %+v; want %v", reply, wantProblem)
			}
			current, err := fixture.api.GetRun(fixture.ctx, protocol.GetRunRequest{RunID: request.RunID})
			if err != nil || current.Status != wantStatus {
				t.Fatalf("SQLite refusal moved the wrong boundary: %+v, %v; want %s", current, err, wantStatus)
			}
			questionCrashDatabase(t, fixture.home, "DROP TRIGGER cancel_ack_refusal")
			reply, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](fixture.ctx, delivery.RunsCancel, request, meta)
			if err != nil || reply == nil || reply.Run.Outcome == nil || reply.Run.Outcome.Type != protocol.OutcomeCanceled ||
				reply.Run.Outcome.Detail != cancelReceiptReason {
				t.Fatalf("same key lost the original cancellation after a SQLite refusal: %+v, %v", reply, err)
			}
			if cut == cancelReceiptWrite {
				got := cancellationResponseJSON(t, reply)
				want := cancellationResponseJSON(t, &protocol.CancelRunResponse{Type: protocol.CancelRunRoot, Run: *current})
				if !bytes.Equal(got, want) {
					t.Fatalf("receipt retry changed the committed terminal result:\ngot  %s\nwant %s", got, want)
				}
			}
		})
	}
}
