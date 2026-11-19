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
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	modernsqlite "modernc.org/sqlite"
)

func TestLiveRootCancellationReplaysItsRefusalAfterCallerTimeout(t *testing.T) {
	testLateCancellationRefusal(t, false)
}

func TestLiveRootCancellationRepairsRefusalReceiptOnShutdown(t *testing.T) {
	testLateCancellationRefusal(t, true)
}

func testLateCancellationRefusal(t *testing.T, refuseWrite bool) {
	t.Helper()
	if !runCancellationFixture(t) {
		return
	}
	completing, commit := make(chan struct{}), make(chan struct{})
	releaseCommit := sync.OnceFunc(func() { close(commit) })
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
			close(completing)
			<-commit
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	entered, finish := make(chan struct{}), make(chan struct{})
	reserved, submit := make(chan struct{}), make(chan struct{})
	writeFailed, returnFailure := make(chan struct{}), make(chan struct{})
	joining := make(chan struct{})
	releaseModel := sync.OnceFunc(func() { close(finish) })
	releaseCancel := sync.OnceFunc(func() { close(submit) })
	releaseFailure := sync.OnceFunc(func() { close(returnFailure) })
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
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: home})
	if err != nil {
		t.Fatal(err)
	}
	host := assembleTestRuntime(t, protocolRuntimeConfig(t, stores, model))
	defer func() {
		releaseCommit()
		releaseCancel()
		releaseModel()
		releaseFailure()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	api, err := protocolHandler(host, home)
	if err != nil {
		t.Fatal(err)
	}
	var replayStore operation.ReplayStore = cancellationReservationGate{stores.Idempotency, reserved, submit}
	if refuseWrite {
		replayStore = cancellationRefusalWriteGate{replayStore, sync.OnceFunc(func() {
			close(writeFailed)
			<-returnFailure
		})}
		host.lifetime.runSegments = observedSegmentShutdown{
			host.lifetime.runSegments, sync.OnceFunc(func() { close(joining) }),
		}
	}
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
			BEGIN SELECT RAISE(ABORT, 'refused original cancellation response'); END`)
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
	waitForRunEvents(t, terminal, "the original natural Run completion")
	run, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID})
	if err != nil || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("the original Run's natural completion was replaced: %+v, %v", run, err)
	}
	history := readWaitingChildScopeLog(t, home)
	if controls := waitingChildHostControls(history, history.Root.String()); controls != 0 {
		t.Fatalf("the completed Run received %d cancellation controls", controls)
	}
	if refuseWrite {
		waitForSignal(t, writeFailed, "SQLite's refusal of the original cancellation response")
		closed := make(chan error, 1)
		go func() { closed <- host.Close() }()
		waitForSignal(t, joining, "shutdown joining the Run owner before flushing the receipt")
		questionCrashDatabase(t, home, "DROP TRIGGER refuse_live_cancellation_response")
		releaseFailure()
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(lifecycleWaitBudget(t)):
			t.Fatal("shutdown did not flush the original cancellation response")
		}
	} else {
		waitForCancellationRefusal(t, ctx, endpoint, request, key)
		if err := host.Close(); err != nil {
			t.Fatal(err)
		}
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

type cancellationRefusalWriteGate struct {
	operation.ReplayStore
	afterFailure func()
}

func (g cancellationRefusalWriteGate) Complete(ctx context.Context, record operation.ReplayRecord) error {
	err := g.ReplayStore.Complete(ctx, record)
	if err != nil {
		g.afterFailure()
	}
	return err
}

type observedSegmentShutdown struct {
	shutdownComponent
	onAwait func()
}

func (s observedSegmentShutdown) AwaitShutdown(ctx context.Context) error {
	s.onAwait()
	return s.shutdownComponent.AwaitShutdown(ctx)
}

func waitForCancellationRefusal(
	t *testing.T,
	ctx context.Context,
	endpoint *delivery.Endpoint,
	request protocol.CancelRunRequest,
	key protocol.CallMetadata,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var pending *protocol.Failure
	for {
		result := endpoint.Invoke(ctx, delivery.RunsCancel, request, key)
		if errors.Is(result.Failure, protocol.ErrRunFinished) {
			return
		}
		if ctx.Err() != nil && pending != nil {
			t.Fatalf("the original key remained in progress after the Run completed: %v", pending)
		}
		if !errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
			t.Fatalf("the original key returned a different result: %+v", result)
		}
		pending = result.Failure
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("the original key remained in progress after the Run completed: %v", result.Failure)
		}
	}
}
