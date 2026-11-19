package bootstrap

import (
	"context"
	"database/sql/driver"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/invocation"
	modernsqlite "modernc.org/sqlite"
)

func TestActiveRootCancellationRetainsItsOriginalControlOwner(t *testing.T) {
	if !runRootCancellationFixture(t) {
		return
	}

	controlWriting, controlWrite, secondControl := make(chan struct{}), make(chan struct{}), make(chan struct{})
	releaseControl := sync.OnceFunc(func() { close(controlWrite) })
	err := modernsqlite.RegisterScalarFunction("test_gate_original_root_cancel_control", 1,
		func(_ *modernsqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			payload, ok := args[0].([]byte)
			if !ok {
				return nil, errors.New("the control write was missing the Scope log")
			}
			history, err := waitingChildScopeLog(payload)
			if err != nil {
				return nil, err
			}
			control, ok := history.Entries[len(history.Entries)-1].Record.(scoperun.ControlRequested)
			if !ok || control.Process != history.Root || control.Kind != scoperun.ControlCancel ||
				control.Origin != scoperun.ControlOriginHost {
				return nil, nil
			}
			switch waitingChildHostControls(history, history.Root.String()) {
			case 1:
				close(controlWriting)
				<-controlWrite
			case 2:
				close(secondControl)
			}
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	modelStarted, modelCanceled, modelReturn := make(chan struct{}), make(chan struct{}), make(chan struct{})
	releaseModel := sync.OnceFunc(func() { close(modelReturn) })
	model := delegateRestartModel{chat.ModelFunc(func(ctx context.Context, _ *chat.Request) (*chat.Response, error) {
		close(modelStarted)
		<-ctx.Done()
		close(modelCanceled)
		<-modelReturn
		return nil, &invocation.Failure{Kind: invocation.FailureNotExecuted, Cause: ctx.Err()}
	})}
	host, api := openProtocolRuntime(t, model)
	defer func() {
		releaseControl()
		releaseModel()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	questionCrashDatabase(t, home, `CREATE TRIGGER gate_original_root_cancel_control BEFORE UPDATE ON execution_trees
		BEGIN SELECT test_gate_original_root_cancel_control(NEW.payload); END`)
	ctx := protocolLifecycleContext(t.Context())
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "cancel a live root after delayed control acknowledgement",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "begin an active turn"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	terminal := collectRunEvents(events)
	waitForSignal(t, modelStarted, "the original model invocation")

	request := protocol.CancelRunRequest{RunID: started.RunID, Reason: activeRootCancelReason}
	originalKey := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	original := make(chan protocol.OperationResult, 1)
	go func() {
		original <- host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, originalKey)
	}()
	waitForSignal(t, controlWriting, "the original Scope control in its SQLite transaction")
	select {
	case reply := <-original:
		if reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrInternalError) {
			t.Fatalf("the first request acknowledged the uncommitted Scope control: %+v", reply)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the first request did not stop waiting for its Scope control")
	}

	releaseControl()
	waitForSignal(t, modelCanceled, "the original committed control reaching the model")
	competing := make(chan protocol.OperationResult, 1)
	go func() {
		key := protocol.CallMetadata{IdempotencyKey: "another-active-root-cancellation"}
		competing <- host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, key)
	}()
	select {
	case reply := <-competing:
		if reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrSessionBusy) {
			t.Errorf("a different command took the original cancellation's place: %+v", reply)
		}
	case <-secondControl:
		t.Error("the competing request submitted a second Scope cancellation")
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the competing request did not reach cancellation arbitration")
	}
	releaseModel()
	waitForRunEvents(t, terminal, "settlement of the original Run")
	history := readWaitingChildScopeLog(t, home)
	if controls := waitingChildHostControls(history, history.Root.String()); controls != 1 {
		t.Errorf("the original Run has %d durable cancellation controls, want one", controls)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER gate_original_root_cancel_control")
	restarted, _ := openProtocolRuntime(t, newReplyStub("the canceled Run must remain settled"))
	defer restarted.Close()
	reply, err := restarted.Endpoint().Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, originalKey)
	if err != nil {
		t.Fatalf("the original cancellation receipt did not survive restart: %v", err)
	}
	if reply.Run.ID != started.RunID || reply.Run.Outcome == nil || reply.Run.Outcome.Type != protocol.OutcomeCanceled ||
		reply.Run.Outcome.Detail != request.Reason {
		t.Fatalf("the original request replayed a different terminal Run: %+v", reply.Run)
	}
}

func TestLiveRootCancellationJoinsAnInFlightPark(t *testing.T) {
	if !runRootCancellationFixture(t) {
		return
	}
	writing, finish := make(chan struct{}), make(chan struct{})
	releasePark := sync.OnceFunc(func() { close(finish) })
	if err := modernsqlite.RegisterScalarFunction("test_gate_park_before_root_cancellation", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			close(writing)
			<-finish
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := newLifecycleModel()
	host, api, stores := openProtocolRuntimeWithStores(t, model)
	reserved, continueCancellation := make(chan struct{}), make(chan struct{})
	releaseCancellation := sync.OnceFunc(func() { close(continueCancellation) })
	defer func() {
		releaseCancellation()
		releasePark()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := protocolLifecycleContext(t.Context())
	request := startCancelableActiveRoot(t, ctx, api, model, home)
	questionCrashDatabase(t, home, `CREATE TRIGGER gate_park_before_root_cancellation BEFORE UPDATE OF state ON runs
		WHEN NEW.state = 'waiting'
		BEGIN SELECT test_gate_park_before_root_cancellation(); END`)
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: parkedCancellationReservationGate{stores.Idempotency, reserved, continueCancellation},
		ValidateStorage:  stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	result := make(chan protocol.OperationResult, 1)
	go func() {
		result <- endpoint.Invoke(ctx, delivery.RunsCancel, request, key)
	}()
	waitForSignal(t, reserved, "the live cancellation's original key reservation")
	close(model.releaseFirstCall)
	waitForSignal(t, writing, "the original interrupt in its SQLite transaction")
	releaseCancellation()
	select {
	case reply := <-result:
		if reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrInternalError) {
			t.Fatalf("the request did not reach its wait deadline while the interrupt was writing: %+v", reply)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the cancellation did not stop waiting for the original interrupt")
	}

	releasePark()
	waitForProtocolRunTerminal(t, ctx, api, request.RunID)
	reply, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, key)
	if err != nil {
		t.Fatalf("the live request lost its parked cancellation receipt: %v", err)
	}
	if reply.Run.ID != request.RunID || reply.Run.Outcome == nil || reply.Run.Outcome.Type != protocol.OutcomeCanceled ||
		reply.Run.Outcome.Detail != request.Reason {
		t.Fatalf("the live request did not cancel its original parked Run: %+v", reply.Run)
	}
	history := readWaitingChildScopeLog(t, home)
	if controls := waitingChildHostControls(history, history.Root.String()); controls != 0 {
		t.Fatalf("the parked cancellation also submitted %d live controls", controls)
	}
}

type parkedCancellationReservationGate struct {
	operation.ReplayStore
	reserved chan<- struct{}
	proceed  <-chan struct{}
}

func (g parkedCancellationReservationGate) Claim(ctx context.Context, key, fingerprint string) (operation.ReplayRecord, bool, error) {
	record, claimed, err := g.ReplayStore.Claim(ctx, key, fingerprint)
	if err == nil && claimed {
		close(g.reserved)
		<-g.proceed
	}
	return record, claimed, err
}

func runRootCancellationFixture(t *testing.T) bool {
	t.Helper()
	const fixture = "FLAME_TEST_ACTIVE_ROOT_CONTROL_ACK"
	if os.Getenv(fixture) == t.Name() {
		return true
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+regexp.QuoteMeta(t.Name())+"$", "-test.v")
	child.Env = append(os.Environ(), fixture+"="+t.Name())
	if out, err := child.CombinedOutput(); err != nil {
		t.Fatalf("the cancellation fixture failed: %v\n%s", err, out)
	}
	return false
}
