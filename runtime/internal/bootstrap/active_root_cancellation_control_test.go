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
	if !runCancellationFixture(t) {
		return
	}

	gate := registerCancelControlGate(t, true)
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
		gate.release()
		releaseModel()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	gate.install(t, home)
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
	waitForSignal(t, gate.writing, "the original Scope control in its SQLite transaction")
	select {
	case reply := <-original:
		if reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrInternalError) {
			t.Fatalf("the first request acknowledged the uncommitted Scope control: %+v", reply)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the first request did not stop waiting for its Scope control")
	}

	gate.release()
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
	case <-gate.repeated:
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
	gate.drop(t, home)
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
	if !runCancellationFixture(t) {
		return
	}
	writing, releasePark := registerParkCommitGate(t)
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := newLifecycleModel()
	host, api, stores := openProtocolRuntimeWithStores(t, model)
	reserved, continueCancellation := make(chan struct{}), make(chan struct{})
	competingReserved, continueCompeting := make(chan struct{}), make(chan struct{})
	releaseCompeting := sync.OnceFunc(func() { close(continueCompeting) })
	releaseCancellation := sync.OnceFunc(func() { close(continueCancellation) })
	defer func() {
		releaseCancellation()
		releaseCompeting()
		releasePark()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := protocolLifecycleContext(t.Context())
	request := startCancelableActiveRoot(t, ctx, api, model, home)
	installParkCommitGate(t, home)
	competing := cancellationReservationGate{stores.Idempotency, competingCancelKey, competingReserved, continueCompeting}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: cancellationReservationGate{competing, activeRootCancelKey, reserved, continueCancellation},
		ValidateStorage:  stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	key := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	competitor := reserveCompetingCancellation(t, ctx, endpoint, request, key, competingReserved)
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

	waitForRunOutcomeAfter(t, ctx, api, request.RunID, protocol.OutcomeCanceled, releasePark)
	assertCompetingCancellationRefused(t, ctx, host.Endpoint(), request, key)
	releaseCompeting()
	waitForCompetingCancellationRefusal(t, competitor)
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

type cancellationReservationGate struct {
	operation.ReplayStore
	key      string
	reserved chan<- struct{}
	proceed  <-chan struct{}
}

func (g cancellationReservationGate) Claim(ctx context.Context, claim operation.ReplayClaim) (operation.ReplayRecord, bool, error) {
	record, claimed, err := g.ReplayStore.Claim(ctx, claim)
	if err == nil && claimed && claim.Key == g.key {
		close(g.reserved)
		<-g.proceed
	}
	return record, claimed, err
}

func registerParkCommitGate(t *testing.T) (<-chan struct{}, func()) {
	t.Helper()
	writing, release := make(chan struct{}), make(chan struct{})
	wait := sync.OnceFunc(func() {
		close(writing)
		<-release
	})
	if err := modernsqlite.RegisterScalarFunction("test_gate_park_before_cancellation", 0,
		func(*modernsqlite.FunctionContext, []driver.Value) (driver.Value, error) {
			wait()
			return nil, nil
		}); err != nil {
		t.Fatal(err)
	}
	return writing, sync.OnceFunc(func() { close(release) })
}

func installParkCommitGate(t *testing.T, home string) {
	t.Helper()
	questionCrashDatabase(t, home, `CREATE TRIGGER gate_park_before_cancellation BEFORE UPDATE OF state ON runs
		WHEN NEW.state = 'waiting'
		BEGIN SELECT test_gate_park_before_cancellation(); END`)
}

type cancelControlGate struct {
	writing  <-chan struct{}
	repeated <-chan struct{}
	release  func()
}

func registerCancelControlGate(t *testing.T, root bool) cancelControlGate {
	t.Helper()
	writing, commit, repeated := make(chan struct{}), make(chan struct{}), make(chan struct{})
	err := modernsqlite.RegisterScalarFunction("test_gate_cancel_control", 1,
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
			if !ok || (control.Process == history.Root) != root || control.Kind != scoperun.ControlCancel ||
				control.Origin != scoperun.ControlOriginHost {
				return nil, nil
			}
			switch waitingChildHostControls(history, control.Process.String()) {
			case 1:
				close(writing)
				<-commit
			case 2:
				close(repeated)
			}
			return nil, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	return cancelControlGate{writing, repeated, sync.OnceFunc(func() { close(commit) })}
}

func (cancelControlGate) install(t *testing.T, home string) {
	t.Helper()
	questionCrashDatabase(t, home, `CREATE TRIGGER gate_original_cancel_control BEFORE UPDATE ON execution_trees
		BEGIN SELECT test_gate_cancel_control(NEW.payload); END`)
}

func (cancelControlGate) drop(t *testing.T, home string) {
	t.Helper()
	questionCrashDatabase(t, home, "DROP TRIGGER gate_original_cancel_control")
}

func runCancellationFixture(t *testing.T) bool {
	t.Helper()
	const fixture = "FLAME_TEST_CANCEL_CONTROL_ACK"
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
