package bootstrap

import (
	"context"
	"database/sql"
	"database/sql/driver"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	localruntime "github.com/Tangerg/flame/packages/local-runtime"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

const activeRootScopeExitEnv = "FLAME_TEST_ACTIVE_ROOT_SCOPE_EXIT"

func TestScopeRootCancellationTransactionCrashHelper(t *testing.T) {
	path := os.Getenv(activeRootScopeExitEnv)
	if path == "" {
		return
	}
	if err := modernsqlite.RegisterScalarFunction("test_exit_during_cancel_run_projection", 1,
		func(_ *modernsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			payload, ok := args[0].([]byte)
			if !ok {
				return nil, errors.New("the Run transaction had no original Scope head")
			}
			log, err := waitingChildScopeLog(payload)
			if err != nil || !hasConfirmedScopeRootCancellation(log) {
				return nil, errors.Join(err, errors.New("the Run transaction did not see the original Scope Exit"))
			}
			os.Exit(commandCrashExit)
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("FLAME_HOME")
	model := newLifecycleModel()
	host, api := openProtocolRuntime(t, model)
	defer host.Close()
	ctx := protocolLifecycleContext(t.Context())
	request := startCancelableActiveRoot(t, ctx, api, model, home)
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	questionCrashDatabase(t, home, fmt.Sprintf(`CREATE TRIGGER crash_scope_cancel_projection BEFORE UPDATE OF outcome ON runs
		WHEN NEW.run_id = '%s' AND NEW.outcome = 'canceled'
		BEGIN SELECT test_exit_during_cancel_run_projection(
			(SELECT payload FROM execution_trees WHERE session_id = NEW.session_id)
		); END`, request.RunID))
	call := host.Endpoint().Invoke(ctx, delivery.RunsCancel, request,
		protocol.CallMetadata{IdempotencyKey: activeRootCancelKey})
	t.Fatalf("the cancellation never reached the transaction cut: %+v", call.Failure)
}

func TestScopeRootCancellationSettlesItsOriginalKeyAfterProcessExit(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	path := filepath.Join(t.TempDir(), "original-cancel-request.json")
	crashCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestScopeRootCancellationTransactionCrashHelper$")
	child.Env = append(os.Environ(), activeRootScopeExitEnv+"="+path)
	out, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("the original Run did not reach the joint Scope Exit and Run transaction: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var request protocol.CancelRunRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatal(err)
	}
	history := readWaitingChildScopeLog(t, home)
	if controls := waitingChildHostControls(history, history.Root.String()); controls != 1 || hasConfirmedScopeRootCancellation(history) {
		t.Fatalf("the Scope Exit escaped the rolled-back Run transaction, with %d accepted controls", controls)
	}

	directory, err := localruntime.DataDirectoryAt(home)
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", directory.DatabasePath())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var state protocol.RunStatus
	var outcome string
	if err := db.QueryRowContext(t.Context(), "SELECT state, outcome FROM runs WHERE run_id = ?", request.RunID).Scan(&state, &outcome); err != nil {
		t.Fatal(err)
	}
	if state != protocol.RunStatusRunning || outcome != "" {
		t.Fatalf("Run escaped its rolled-back Scope transaction: state=%s, outcome=%q", state, outcome)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER crash_scope_cancel_projection")

	var calls atomic.Int32
	reply := newReplyStub("only a new Run can dispatch after recovery")
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		return reply.Call(ctx, request)
	})}
	assertAbandonedCancellationRollsBackOnReceiptRefusal(t, home, request, model)
	host, api := openProtocolRuntime(t, model)
	defer host.Close()
	ctx := protocolLifecycleContext(t.Context())
	value, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: request.RunID})
	if err != nil || value.Outcome == nil || value.Outcome.Type != protocol.OutcomeLost {
		t.Fatalf("the Run had no confirmed terminal result after its writer died: %+v, %v", value, err)
	}
	key := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	result := host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, key)
	if !errors.Is(result.Failure, protocol.ErrInternalError) {
		t.Fatalf("recovery did not settle the original cancellation as unconfirmed: %+v", result)
	}
	if err := api.DeleteSession(ctx, value.SessionID); err != nil {
		t.Fatal(err)
	}
	if replayed := host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, key); !errors.Is(replayed.Failure, protocol.ErrInternalError) {
		t.Fatalf("replay lost the original cancellation result after Session deletion: %+v", replayed)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("recovery dispatched %d model calls", got)
	}
}

func assertAbandonedCancellationRollsBackOnReceiptRefusal(t *testing.T, home string, request protocol.CancelRunRequest, model chat.Model) {
	t.Helper()
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	host := assembleTestRuntime(t, protocolRuntimeConfig(t, stores, model))
	defer func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	const refuseReceipt = "original cancellation receipt write refused"
	questionCrashDatabase(t, home, fmt.Sprintf(`CREATE TRIGGER refuse_abandoned_cancellation BEFORE UPDATE OF kind ON idempotency_records
		WHEN OLD.key = '%s' AND NEW.kind <> OLD.kind
		BEGIN SELECT RAISE(ABORT, '%s'); END`, activeRootCancelKey, refuseReceipt))
	defer questionCrashDatabase(t, home, "DROP TRIGGER refuse_abandoned_cancellation")

	api, err := protocolHandler(host, home)
	if err != nil {
		t.Fatal(err)
	}
	err = startTestRuntimeWithReplayStore(t, host, api, stores, stores.Idempotency)
	if err == nil || !strings.Contains(err.Error(), refuseReceipt) {
		t.Fatalf("startup did not observe the refusal of the original receipt: %v", err)
	}
	ctx := protocolLifecycleContext(t.Context())
	value, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: request.RunID})
	if err != nil || value.Status != protocol.RunStatusRunning || value.Outcome != nil {
		t.Fatalf("Run loss escaped its rolled-back receipt transaction: %+v, %v", value, err)
	}
	history := readWaitingChildScopeLog(t, home)
	if controls := waitingChildHostControls(history, history.Root.String()); controls != 1 || hasConfirmedScopeRootCancellation(history) {
		t.Fatalf("refused recovery changed the original Scope evidence, with %d controls", controls)
	}
}

func hasConfirmedScopeRootCancellation(history scoperun.Log) bool {
	for _, entry := range history.Entries {
		if exit, ok := entry.Record.(scoperun.Exited); ok && exit.Process == history.Root &&
			exit.Termination.Cause() == scoperun.TerminationCauseHostCancellation {
			return true
		}
	}
	return false
}
