package bootstrap

import (
	"context"
	"database/sql/driver"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

func TestLiveRootCancellationReplaysItsRefusalAfterCallerTimeout(t *testing.T) {
	testLateCancellationRefusal(t, false)
}

func TestLiveRootCancellationRollsBackNaturalExitOnReceiptRefusal(t *testing.T) {
	testLateCancellationRefusal(t, true)
}

func testLateCancellationRefusal(t *testing.T, refuseWrite bool) {
	t.Helper()
	if !runCancellationFixture(t) {
		return
	}
	completing, commit := make(chan struct{}), make(chan struct{})
	releaseCommit := sync.OnceFunc(func() { close(commit) })
	holdCommit := sync.OnceFunc(func() {
		close(completing)
		<-commit
	})
	err := modernsqlite.RegisterScalarFunction("test_gate_natural_exit_before_cancel", 1,
		func(_ *modernsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			payload, ok := args[0].([]byte)
			if !ok {
				return nil, errors.New("the Run transaction had no Scope head")
			}
			history, err := waitingChildScopeLog(payload)
			if err != nil || !scopeRootCompleted(history) {
				return nil, errors.Join(err, errors.New("the Scope root did not complete in the Run transaction"))
			}
			holdCommit()
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	entered, finish := make(chan struct{}), make(chan struct{})
	reserved, submit := make(chan struct{}), make(chan struct{})
	writeFailed := make(chan struct{})
	releaseModel := sync.OnceFunc(func() { close(finish) })
	releaseCancel := sync.OnceFunc(func() { close(submit) })
	if refuseWrite {
		reportRefusal := sync.OnceFunc(func() { close(writeFailed) })
		if err := modernsqlite.RegisterScalarFunction("test_record_natural_cancellation_refusal", 0,
			func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
				reportRefusal()
				return nil, nil
			}); err != nil {
			t.Fatal(err)
		}
	}
	reply := newReplyStub(rootCompletionReply)
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		close(entered)
		select {
		case <-finish:
			return reply.Call(ctx, request)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})}
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home, ValidateDirectory: persistencefixture.UnleasedDirectory})
	if err != nil {
		t.Fatal(err)
	}
	host := assembleTestRuntime(t, protocolRuntimeConfig(t, stores, model))
	defer func() {
		releaseCommit()
		releaseCancel()
		releaseModel()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	api, err := protocolHandler(host, home)
	if err != nil {
		t.Fatal(err)
	}
	replayStore := cancellationReservationGate{stores.Idempotency, activeRootCancelKey, reserved, submit}
	if err := startTestRuntimeWithReplayStore(t, host, api, stores, replayStore); err != nil {
		t.Fatal(err)
	}
	endpoint := host.Endpoint()
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "cancellation racing natural completion",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "complete the original Run"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal := collectRunEvents(events)
	waitForSignal(t, entered, "the original Run's model")
	questionCrashDatabase(t, home, `CREATE TRIGGER gate_natural_exit_before_cancel BEFORE UPDATE OF outcome ON runs
		WHEN NEW.outcome = 'completed'
		BEGIN SELECT test_gate_natural_exit_before_cancel(
			(SELECT payload FROM execution_trees WHERE session_id = NEW.session_id)
		); END`)
	if refuseWrite {
		questionCrashDatabase(t, home, `CREATE TRIGGER refuse_live_cancellation_response
			BEFORE UPDATE OF kind ON idempotency_records WHEN NEW.kind <> OLD.kind
			BEGIN
				SELECT test_record_natural_cancellation_refusal();
				SELECT RAISE(ABORT, 'refused original cancellation response');
			END`)
	}
	request := protocol.CancelRunRequest{RunID: started.RunID, Reason: activeRootCancelReason}
	key := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	original := make(chan protocol.OperationResult, 1)
	go func() { original <- endpoint.Invoke(ctx, delivery.RunsCancel, request, key) }()
	waitForSignal(t, reserved, "the original cancellation key reservation")
	releaseModel()
	waitForSignal(t, completing, "the natural Scope Exit in its SQLite transaction")
	releaseCancel()
	select {
	case result := <-original:
		if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInternalError) {
			t.Fatalf("the original caller did not time out while the Run transaction was open: %+v", result)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the original caller did not stop waiting")
	}

	releaseCommit()
	if refuseWrite {
		waitForSignal(t, writeFailed, "SQLite's refusal of the original cancellation response")
		value, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
		if err != nil || value.Status != protocol.RunStatusRunning || value.Outcome != nil {
			t.Fatalf("the Run's terminal escaped the refused receipt transaction: %+v, %v", value, err)
		}
		if scopeRootCompleted(readWaitingChildScopeLog(t, home)) {
			t.Fatal("the Scope Exit escaped the refused receipt transaction")
		}
		questionCrashDatabase(t, home, "DROP TRIGGER refuse_live_cancellation_response")
	}
	waitForRunEvents(t, terminal, "the original natural Run completion")
	run, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("the original Run's natural completion was replaced: %+v, %v", run, err)
	}
	history := readWaitingChildScopeLog(t, home)
	if controls := waitingChildHostControls(history, history.Root.String()); controls != 0 {
		t.Fatalf("the completed Run received %d cancellation controls", controls)
	}
	if result := endpoint.Invoke(ctx, delivery.RunsCancel, request, key); !errors.Is(result.Failure, protocol.ErrRunFinished) {
		t.Fatalf("the terminal Run did not settle the original key: %+v", result)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER gate_natural_exit_before_cancel")
	restarted, cold := openProtocolRuntime(t, newReplyStub("the original Run is already completed"))
	defer restarted.Close()
	if err := cold.DeleteSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if result := restarted.Endpoint().Invoke(ctx, delivery.RunsCancel, request, key); !errors.Is(result.Failure, protocol.ErrRunFinished) {
		t.Fatalf("the original key lost its refusal across restart and Session deletion: %+v", result)
	}
	newKey := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey + "-after-delete"}
	if result := restarted.Endpoint().Invoke(ctx, delivery.RunsCancel, request, newKey); !errors.Is(result.Failure, protocol.ErrRunNotFound) {
		t.Fatalf("a new key did not report the deleted Run: %+v", result)
	}
}
