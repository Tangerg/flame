package bootstrap

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/invocation"
)

const (
	activeChildCancelCutEnv = "FLAME_TEST_ACTIVE_CHILD_CANCEL_RECEIPT"
	activeChildCancelKey    = "original-active-child-cancellation"
	activeChildCancelReason = "Cancel one currently running delegate"
	activeChildInstruction  = "wait for the caller to cancel this child"
)

type activeChildCancelCrashStore struct {
	operation.ReplayStore
	path string
}

func (s activeChildCancelCrashStore) Complete(ctx context.Context, record operation.ReplayRecord) (operation.ReplayRecord, error) {
	committed, found, err := s.ReplayStore.Lookup(ctx, record.Key, record.Fingerprint)
	if err != nil {
		return operation.ReplayRecord{}, err
	}
	if !found || len(committed.Settlement) == 0 || !bytes.Equal(committed.Settlement, record.Settlement) {
		return operation.ReplayRecord{}, errors.New("the canceled child was missing its original staged receipt")
	}
	if err := os.WriteFile(s.path, record.Payload, 0o600); err != nil {
		return operation.ReplayRecord{}, err
	}
	os.Exit(commandCrashExit)
	return operation.ReplayRecord{}, nil
}

func cancelableActiveChildModel(entered chan<- struct{}) chat.Model {
	return activeChildCancellationModel(func(ctx context.Context) (*chat.Response, error) {
		close(entered)
		<-ctx.Done()
		return nil, &invocation.Failure{Kind: invocation.FailureNotExecuted, Cause: ctx.Err()}
	})
}

func activeChildCancellationModel(child func(context.Context) (*chat.Response, error)) chat.Model {
	return delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		var gotResult bool
		for _, message := range request.Messages {
			if message.Role == chat.RoleUser && strings.Contains(message.Text(), activeChildInstruction) {
				return child(ctx)
			}
			gotResult = gotResult || message.Role == chat.RoleTool
		}
		content := chat.NewAssistantMessage(chat.NewTextPart("parent completed after the child"))
		reason := chat.FinishReasonStop
		if len(request.Tools) > 0 && !gotResult {
			args, err := json.Marshal(struct {
				Summary      string `json:"summary"`
				Instructions string `json:"instructions"`
			}{Summary: "a single delegate", Instructions: activeChildInstruction})
			if err != nil {
				return nil, err
			}
			content = chat.NewAssistantMessage(chat.NewToolCallPart(chat.ToolCall{
				ID: "delegate", Name: "delegate_task", Arguments: string(args),
			}))
			reason = chat.FinishReasonToolCalls
		}
		return chat.NewResponse(&chat.Output{Message: &content, FinishReason: reason},
			&chat.ResponseMetadata{Model: "claude-test", Usage: &chat.Usage{InputTokens: 2, OutputTokens: 1}})
	})}
}

func TestActiveChildCancelReceiptCrashHelper(t *testing.T) {
	path := os.Getenv(activeChildCancelCutEnv)
	if path == "" {
		return
	}
	home := os.Getenv("FLAME_HOME")
	entered := make(chan struct{})
	host, api, stores := openProtocolRuntimeWithStores(t, cancelableActiveChildModel(entered))
	defer host.Close()
	meta := waitingChildCancellationMeta()
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	child := startCancelableActiveChild(t, ctx, api, entered, home)
	reserved, proceed := make(chan struct{}), make(chan struct{})
	defer close(proceed)
	gate := cancellationReservationGate{stores.Idempotency, competingCancelKey, reserved, proceed}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: activeChildCancelCrashStore{ReplayStore: gate, path: path},
		ValidateStorage:  stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CancelRunRequest{RunID: child.ID, Reason: activeChildCancelReason}
	options := protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: activeChildCancelKey}
	reserveCompetingCancellation(t, ctx, endpoint, request, options, reserved)
	reply := endpoint.Invoke(ctx, delivery.RunsCancel, request, options)
	t.Fatalf("the active child did not reach the receipt cut: %+v", reply.Failure)
}

func startCancelableActiveChild(t *testing.T, ctx context.Context, api *delivery.Handler, entered <-chan struct{}, home string) protocol.RunRef {
	t.Helper()
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "cancel a live delegate",
	})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "start the delegate"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = collectRunEvents(events)
	waitForSignal(t, entered, "the active child model")
	listed, err := api.ListRuns(ctx, protocol.ListRunsRequest{SessionID: session.ID, IncludeDescendants: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range listed.Data {
		if value.ParentRunID == started.RunID && value.Status == protocol.RunStatusRunning {
			return value
		}
	}
	t.Fatalf("no original running child in %+v", listed.Data)
	return protocol.RunRef{}
}

func TestActiveChildCancellationRecoversItsOriginalReceipt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	path := filepath.Join(t.TempDir(), "original-active-child-cancel.json")
	crashCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestActiveChildCancelReceiptCrashHelper$")
	child.Env = append(os.Environ(), activeChildCancelCutEnv+"="+path)
	out, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("the original child didn't reach its receipt boundary: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var reply struct {
		Value *protocol.CancelRunResponse `json:"value"`
	}
	if err := json.Unmarshal(data, &reply); err != nil {
		t.Fatal(err)
	}
	want := reply.Value
	if want == nil || want.RootRun == nil || want.Type != protocol.CancelRunChild ||
		want.Run.Outcome == nil || want.Run.Outcome.Type != protocol.OutcomeCanceled ||
		want.Run.Outcome.Detail != activeChildCancelReason {
		t.Fatalf("the original result was not a confirmed child cancellation: %s", data)
	}
	if controls := childHostCancelControls(readWaitingChildScopeLog(t, home)); controls != 1 {
		t.Fatalf("the canceled child has %d host controls, want one", controls)
	}
	host, api := openProtocolRuntime(t, newReplyStub("never re-run the old child"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	meta := waitingChildCancellationMeta()
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	terminal, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: want.Run.ID})
	if err != nil || terminal.Outcome == nil || terminal.Outcome.Type != protocol.OutcomeCanceled {
		t.Fatalf("the canceled child did not survive restart: %+v, %v", terminal, err)
	}
	root, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: want.RootRun.ID})
	if err != nil || root.Status != protocol.RunStatusFinished || root.Status == want.RootRun.Status {
		t.Fatalf("the parent did not move beyond the original cancellation response: %+v, %v", root, err)
	}
	if err := api.DeleteSession(ctx, terminal.SessionID); err != nil {
		t.Fatal(err)
	}
	request := protocol.CancelRunRequest{RunID: want.Run.ID, Reason: activeChildCancelReason}
	options := protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: activeChildCancelKey}
	endpoint := host.Endpoint()
	assertCompetingCancellationRefused(t, ctx, endpoint, request, options)
	limited := protocol.CallMetadata{IdempotencyKey: activeChildCancelKey}
	if reply := endpoint.Invoke(ctx, delivery.RunsCancel, request, limited); reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("pending child receipt replay bypassed capability negotiation: %+v", reply)
	}
	got, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, options)
	if err != nil {
		t.Fatalf("the original child receipt was lost: %v", err)
	}
	if !bytes.Equal(cancellationResponseJSON(t, got), cancellationResponseJSON(t, want)) {
		t.Fatalf("replay changed the original child or parent: got %+v, want %+v", got, want)
	}
	if reply := endpoint.Invoke(ctx, delivery.RunsCancel, request, limited); reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("completed child receipt replay bypassed capability negotiation: %+v", reply)
	}
}

func TestActiveChildCancellationKeepsReceiptWithItsSpawningItem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	entered := make(chan struct{})
	host, api, stores := openProtocolRuntimeWithStores(t, cancelableActiveChildModel(entered))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	meta := waitingChildCancellationMeta()
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	child := startCancelableActiveChild(t, ctx, api, entered, home)
	questionCrashDatabase(t, home, `CREATE TRIGGER refuse_active_child_cancel_receipt BEFORE UPDATE OF payload ON idempotency_records
		WHEN NEW.kind = OLD.kind AND length(NEW.payload) > 0
		BEGIN SELECT RAISE(ABORT, 'active child cancellation receipt refused'); END`)
	refusal := &cancellationReceiptRefusalStore{ReplayStore: stores.Idempotency, refused: make(chan struct{})}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: refusal, ValidateStorage: stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	request := protocol.CancelRunRequest{RunID: child.ID, Reason: activeChildCancelReason}
	options := protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: activeChildCancelKey}
	replies := make(chan protocol.OperationResult, 1)
	go func() { replies <- endpoint.Invoke(ctx, delivery.RunsCancel, request, options) }()
	waitForSignal(t, refusal.refused, "SQLite refusal of the active child cancellation receipt")
	items, err := api.ListItems(ctx, protocol.ListItemsRequest{
		Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, SessionID: child.SessionID, RunID: child.ParentRunID},
	})
	if err != nil {
		t.Fatal(err)
	}
	var parentItem *protocol.Item
	for _, item := range items.Data {
		if item.ID == child.SpawnedByItemID {
			parentItem = &item
			break
		}
	}
	if parentItem == nil || parentItem.Status != protocol.ItemStatusRunning {
		t.Fatalf("the spawning Item escaped its rolled-back receipt transaction: %+v", parentItem)
	}
	select {
	case result := <-replies:
		if result.Failure == nil || !errors.Is(result.Failure, protocol.ErrInternalError) {
			t.Fatalf("the first request acknowledged an unsettled child cancellation: %+v", result)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the first request did not reach its cleanup deadline")
	}
	if pending := endpoint.Invoke(ctx, delivery.RunsCancel, request, options); pending.Failure == nil || !errors.Is(pending.Failure, protocol.ErrIdempotencyInProgress) {
		t.Fatalf("the first request replaced the pending child cancellation: %+v", pending)
	}
	questionCrashDatabase(t, home, "DROP TRIGGER refuse_active_child_cancel_receipt")
	ctx, cancel := context.WithTimeout(ctx, lifecycleWaitBudget(t))
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		result := endpoint.Invoke(ctx, delivery.RunsCancel, request, options)
		if result.Failure == nil {
			reply := result.Value.(*protocol.CancelRunResponse)
			if reply.Run.ID != child.ID || reply.Run.Outcome == nil || reply.Run.Outcome.Type != protocol.OutcomeCanceled ||
				reply.RootRun == nil || reply.RootRun.ID != child.RootRunID || reply.RootRun.Status != protocol.RunStatusRunning {
				t.Fatalf("unexpected original child or parent snapshot after retry: %+v", reply)
			}
			return
		}
		if !errors.Is(result.Failure, protocol.ErrIdempotencyInProgress) {
			t.Fatal(result.Failure)
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("the original child cancellation did not settle after SQLite accepted its receipt")
		}
	}
}
