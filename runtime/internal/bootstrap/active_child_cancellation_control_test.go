package bootstrap

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	scoperun "github.com/Tangerg/scope/agent/run"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/invocation"
)

func TestActiveChildCancellationRetainsItsOriginalControlOwner(t *testing.T) {
	if !runCancellationFixture(t) {
		return
	}
	gate := registerCancelControlGate(t, false)
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	modelStarted, modelCanceled, modelReturn := make(chan struct{}), make(chan struct{}), make(chan struct{})
	releaseModel := sync.OnceFunc(func() { close(modelReturn) })
	model := activeChildCancellationModel(func(ctx context.Context) (*chat.Response, error) {
		close(modelStarted)
		<-ctx.Done()
		close(modelCanceled)
		<-modelReturn
		return nil, &invocation.Failure{Kind: invocation.FailureNotExecuted, Cause: ctx.Err()}
	})
	host, api := openProtocolRuntime(t, model)
	defer func() {
		gate.release()
		releaseModel()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	gate.install(t, home)
	meta := waitingChildCancellationMeta()
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	child := startCancelableActiveChild(t, ctx, api, modelStarted, home)
	request := protocol.CancelRunRequest{RunID: child.ID, Reason: activeChildCancelReason}
	originalKey := protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: activeChildCancelKey}
	original := make(chan protocol.OperationResult, 1)
	go func() {
		original <- host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, originalKey)
	}()
	waitForSignal(t, gate.writing, "the original child cancellation in its SQLite transaction")
	select {
	case reply := <-original:
		if reply.Failure == nil {
			t.Fatalf("the first request acknowledged the uncommitted child control: %+v", reply)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the first request did not stop waiting for its child control")
	}

	gate.release()
	waitForSignal(t, modelCanceled, "the committed child control reaching the model")
	competing := make(chan protocol.OperationResult, 1)
	go func() {
		key := protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: "another-active-child-cancellation"}
		competing <- host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, key)
	}()
	select {
	case reply := <-competing:
		if reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrSessionBusy) {
			t.Errorf("a different request took over the original child cancellation: %+v", reply)
		}
	case <-gate.repeated:
		t.Error("the competing request submitted a second child cancellation")
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the competing request did not reach the child cancellation boundary")
	}
	waitForRunOutcomeAfter(t, ctx, api, child.RootRunID, protocol.OutcomeCompleted, releaseModel)
	if controls := childHostCancelControls(readWaitingChildScopeLog(t, home)); controls != 1 {
		t.Errorf("the child has %d durable cancellation controls, want one", controls)
	}
	if err := host.Close(); err != nil {
		t.Fatal(err)
	}
	gate.drop(t, home)
	restarted, cold := openProtocolRuntime(t, newReplyStub("the canceled child is already terminal"))
	defer restarted.Close()
	reply, err := restarted.Endpoint().Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, originalKey)
	if err != nil {
		t.Errorf("the original child receipt did not survive restart: %v", err)
	} else if reply.Run.ID != child.ID || reply.Run.Outcome == nil || reply.Run.Outcome.Type != protocol.OutcomeCanceled ||
		reply.Run.Outcome.Detail != request.Reason || reply.RootRun == nil ||
		reply.RootRun.ID != child.RootRunID || reply.RootRun.Status != protocol.RunStatusRunning {
		t.Errorf("the original request replayed different child or root snapshots: %+v", reply)
	}
	items, err := cold.ListItems(ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: child.ParentRunID},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items.Data {
		if item.ID == child.SpawnedByItemID {
			if item.Status != protocol.ItemStatusIncomplete || item.Error == nil ||
				item.Error.Type != protocol.ProblemChildRunCanceled || item.Error.Detail != request.Reason {
				t.Errorf("the original parent Tool Item did not record its child cancellation: %+v", item)
			}
			return
		}
	}
	t.Fatal("the original parent Tool Item did not survive restart")
}

func TestLiveChildCancellationJoinsAnInFlightPark(t *testing.T) {
	if !runCancellationFixture(t) {
		return
	}
	writing, releasePark := registerParkCommitGate(t)
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	modelStarted, ask, reserved, proceed := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	releaseQuestion := sync.OnceFunc(func() { close(ask) })
	releaseCancellation := sync.OnceFunc(func() { close(proceed) })
	model := activeChildCancellationModel(func(ctx context.Context) (*chat.Response, error) {
		close(modelStarted)
		select {
		case <-ask:
			return toolCallResponse(chat.ToolCall{
				ID: "child_question", Name: string(builtin.AskUser),
				Arguments: `{"questions":[{"question":"Keep the child running?"}]}`,
			}), nil
		case <-ctx.Done():
			return nil, &invocation.Failure{Kind: invocation.FailureNotExecuted, Cause: ctx.Err()}
		}
	})
	host, api, stores := openProtocolRuntimeWithStores(t, model)
	competingReserved, continueCompeting := make(chan struct{}), make(chan struct{})
	releaseCompeting := sync.OnceFunc(func() { close(continueCompeting) })
	defer func() {
		releaseCancellation()
		releaseCompeting()
		releasePark()
		releaseQuestion()
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	}()
	meta := waitingChildCancellationMeta()
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	child := startCancelableActiveChild(t, ctx, api, modelStarted, home)
	installParkCommitGate(t, home)
	competing := cancellationReservationGate{stores.Idempotency, competingCancelKey, competingReserved, continueCompeting}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: cancellationReservationGate{competing, activeChildCancelKey, reserved, proceed},
		ValidateStorage:  stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CancelRunRequest{RunID: child.ID, Reason: activeChildCancelReason}
	key := protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: activeChildCancelKey}
	competitor := reserveCompetingCancellation(t, ctx, endpoint, request, key, competingReserved)
	result := make(chan protocol.OperationResult, 1)
	go func() { result <- endpoint.Invoke(ctx, delivery.RunsCancel, request, key) }()
	waitForSignal(t, reserved, "the original child cancellation reservation")
	releaseQuestion()
	waitForSignal(t, writing, "the child interrupt writing its original park")
	releaseCancellation()
	select {
	case reply := <-result:
		if reply.Failure == nil {
			t.Fatalf("the original request acknowledged an unsettled parked child: %+v", reply)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the original request did not stop waiting for the park transaction")
	}

	waitForRunOutcomeAfter(t, ctx, api, child.RootRunID, protocol.OutcomeCompleted, releasePark)
	assertCompetingCancellationRefused(t, ctx, host.Endpoint(), request, key)
	releaseCompeting()
	waitForCompetingCancellationRefusal(t, competitor)
	reply, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, key)
	if err != nil {
		t.Fatalf("the original live request lost its parked child receipt: %v", err)
	}
	if reply.Run.ID != child.ID || reply.Run.Outcome == nil || reply.Run.Outcome.Type != protocol.OutcomeCanceled ||
		reply.Run.Outcome.Detail != request.Reason || reply.RootRun == nil || reply.RootRun.ID != child.RootRunID {
		t.Fatalf("the live request did not cancel its original parked child: %+v", reply)
	}
	if controls := childHostCancelControls(readWaitingChildScopeLog(t, home)); controls != 1 {
		t.Fatalf("live-to-parked cancellation wrote %d child controls, want one", controls)
	}
}

func waitForRunOutcomeAfter(t *testing.T, ctx context.Context, api *delivery.Handler, runID string, outcome protocol.RunOutcomeType, release func()) {
	t.Helper()
	watching, cancel := context.WithTimeout(ctx, lifecycleWaitBudget(t))
	defer cancel()
	_, changes, err := api.SubscribeRuntime(watching, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicRunsChanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	release()
	for event, err := range changes {
		if err != nil {
			t.Fatal(err)
		}
		if event.Type != protocol.RuntimeResync &&
			(event.Type != protocol.RuntimeRunsChanged || !slices.Contains(event.RunIDs, runID)) {
			continue
		}
		root, err := api.GetRun(watching, protocol.GetRunRequest{RunID: runID})
		if err != nil {
			t.Fatal(err)
		}
		if root.Status == protocol.RunStatusFinished {
			if root.Outcome == nil || root.Outcome.Type != outcome {
				t.Fatalf("Run did not finish as %s: %+v", outcome, root)
			}
			return
		}
	}
	root, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: runID})
	t.Fatalf("the Run's terminal notification was not observed: %v, Run = %+v, read = %v", watching.Err(), root, err)
}

func childHostCancelControls(history scoperun.Log) int {
	var controls int
	for _, entry := range history.Entries {
		if control, ok := entry.Record.(scoperun.ControlRequested); ok && control.Process != history.Root &&
			control.Origin == scoperun.ControlOriginHost && control.Kind == scoperun.ControlCancel {
			controls++
		}
	}
	return controls
}
