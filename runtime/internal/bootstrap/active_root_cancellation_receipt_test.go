package bootstrap

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

const (
	activeRootCancelCutEnv = "FLAME_TEST_ACTIVE_ROOT_CANCEL_RECEIPT"
	activeRootCancelKey    = "original-active-root-cancellation"
	activeRootCancelReason = "Cancel the active root at its prior Scope control"
	competingCancelKey     = "competing-cancellation-before-arbitration"
)

type activeRootCancelCrashStore struct {
	operation.ReplayStore
	path string
}

func reserveCompetingCancellation(t *testing.T, ctx context.Context, endpoint *delivery.Endpoint, request protocol.CancelRunRequest, options protocol.CallMetadata, reserved <-chan struct{}) <-chan protocol.OperationResult {
	t.Helper()
	options.IdempotencyKey = competingCancelKey
	done := make(chan protocol.OperationResult, 1)
	go func() { done <- endpoint.Invoke(ctx, delivery.RunsCancel, request, options) }()
	waitForSignal(t, reserved, "the competing cancellation's durable key reservation")
	return done
}

func waitForCompetingCancellationRefusal(t *testing.T, done <-chan protocol.OperationResult) {
	t.Helper()
	select {
	case reply := <-done:
		if !errors.Is(reply.Failure, protocol.ErrRunFinished) {
			t.Fatalf("the competing cancellation returned a different refusal: %+v", reply)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the competing cancellation did not return its committed refusal")
	}
}

func assertCompetingCancellationRefused(t *testing.T, ctx context.Context, endpoint *delivery.Endpoint, request protocol.CancelRunRequest, options protocol.CallMetadata) {
	t.Helper()
	options.IdempotencyKey = competingCancelKey
	reply := endpoint.Invoke(ctx, delivery.RunsCancel, request, options)
	if !errors.Is(reply.Failure, protocol.ErrRunFinished) {
		t.Fatalf("the competing cancellation's refusal was lost: %+v", reply)
	}
}

func (s activeRootCancelCrashStore) Complete(ctx context.Context, record operation.ReplayRecord) (operation.ReplayRecord, error) {
	prior, found, err := s.ReplayStore.Lookup(ctx, record.Key, record.Fingerprint)
	if err != nil {
		return operation.ReplayRecord{}, err
	}
	if !found || len(prior.Settlement) == 0 || !bytes.Equal(prior.Settlement, record.Settlement) {
		return operation.ReplayRecord{}, errors.New("the canceled Run was missing its original staged receipt")
	}
	if err := os.WriteFile(s.path, record.Payload, 0o600); err != nil {
		return operation.ReplayRecord{}, err
	}
	os.Exit(commandCrashExit)
	return operation.ReplayRecord{}, nil
}

func TestActiveRootCancelReceiptCrashHelper(t *testing.T) {
	path := os.Getenv(activeRootCancelCutEnv)
	if path == "" {
		return
	}
	home := os.Getenv("FLAME_HOME")
	model := newLifecycleModel()
	host, api, stores := openProtocolRuntimeWithStores(t, model)
	defer host.Close()
	ctx := protocolLifecycleContext(t.Context())
	request := startCancelableActiveRoot(t, ctx, api, model, home)
	reserved, proceed := make(chan struct{}), make(chan struct{})
	defer close(proceed)
	gate := cancellationReservationGate{stores.Idempotency, competingCancelKey, reserved, proceed}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: activeRootCancelCrashStore{ReplayStore: gate, path: path},
		ValidateStorage:  stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	options := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	reserveCompetingCancellation(t, ctx, endpoint, request, options, reserved)
	result := endpoint.Invoke(ctx, delivery.RunsCancel, request, options)
	t.Fatalf("the active root did not reach the receipt cut: %+v", result.Failure)
}

func TestActiveRootCancellationRecoversItsOriginalReceipt(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	path := filepath.Join(t.TempDir(), "active-root-cancel-receipt.json")
	crashCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestActiveRootCancelReceiptCrashHelper$")
	child.Env = append(os.Environ(), activeRootCancelCutEnv+"="+path)
	out, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("the original root didn't reach its receipt cut: %v\n%s", err, out)
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
	original := reply.Value
	if original == nil || original.Type != protocol.CancelRunRoot || original.Run.Outcome == nil ||
		original.Run.Outcome.Type != protocol.OutcomeCanceled || original.Run.Outcome.Detail != activeRootCancelReason {
		t.Fatalf("the original live Run was not confirmed canceled: %s", data)
	}
	history := readWaitingChildScopeLog(t, home)
	if !hasConfirmedScopeRootCancellation(history) {
		t.Fatal("the original canceled Run was published without its confirmed Scope Exit")
	}
	if controls := waitingChildHostControls(history, history.Root.String()); controls != 1 {
		t.Fatalf("the canceled root has %d host controls, want one", controls)
	}

	host, api := openProtocolRuntime(t, newReplyStub("the original Run already finished"))
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	terminal, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: original.Run.ID})
	if err != nil || terminal.Outcome == nil || terminal.Outcome.Type != protocol.OutcomeCanceled ||
		terminal.Outcome.Detail != activeRootCancelReason {
		t.Fatalf("the canceled outcome didn't survive restart: %+v, %v", terminal, err)
	}
	if err := api.DeleteSession(ctx, terminal.SessionID); err != nil {
		t.Fatal(err)
	}

	request := protocol.CancelRunRequest{RunID: original.Run.ID, Reason: activeRootCancelReason}
	options := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	assertCompetingCancellationRefused(t, ctx, host.Endpoint(), request, options)
	for range 2 {
		replayed, err := host.Endpoint().Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, options)
		if err != nil {
			t.Fatalf("the original live cancellation did not replay: %v", err)
		}
		got, want := cancellationResponseJSON(t, replayed), cancellationResponseJSON(t, original)
		if !bytes.Equal(got, want) {
			t.Fatalf("replay changed the original canceled response:\ngot  %s\nwant %s", got, want)
		}
	}
	request.Reason = "another cancellation"
	if different := host.Endpoint().Invoke(ctx, delivery.RunsCancel, request, options); different.Failure == nil || !errors.Is(different.Failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("another command reused the original active cancellation key: %+v", different)
	}
}

type cancellationReceiptRefusalStore struct {
	operation.ReplayStore
	refused chan struct{}
	once    sync.Once
}

func (s *cancellationReceiptRefusalStore) StageSettlement(ctx context.Context, record operation.ReplayRecord) error {
	err := s.ReplayStore.StageSettlement(ctx, record)
	if err != nil {
		s.once.Do(func() { close(s.refused) })
	}
	return err
}

func TestActiveRootCancellationCommitsItsReceiptWithTheRun(t *testing.T) {
	t.Run("request still waiting", func(t *testing.T) { testActiveRootCancellationReceiptRefusal(t, false) })
	t.Run("original request timed out", func(t *testing.T) { testActiveRootCancellationReceiptRefusal(t, true) })
}

func testActiveRootCancellationReceiptRefusal(t *testing.T, waitForTimeout bool) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	model := newLifecycleModel()
	host, api, stores := openProtocolRuntimeWithStores(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := protocolLifecycleContext(t.Context())
	request := startCancelableActiveRoot(t, ctx, api, model, home)
	questionCrashDatabase(t, home, `CREATE TRIGGER refuse_live_cancel_receipt BEFORE UPDATE OF payload ON idempotency_records
		WHEN NEW.kind = OLD.kind AND length(NEW.payload) > 0
		BEGIN SELECT RAISE(ABORT, 'active cancellation receipt refused'); END`)
	refusal := &cancellationReceiptRefusalStore{ReplayStore: stores.Idempotency, refused: make(chan struct{})}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: refusal, ValidateStorage: stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	options := protocol.CallMetadata{IdempotencyKey: activeRootCancelKey}
	result := make(chan protocol.OperationResult, 1)
	go func() {
		result <- endpoint.Invoke(ctx, delivery.RunsCancel, request, options)
	}()
	waitForSignal(t, refusal.refused, "SQLite refusal of the original cancellation receipt")
	current, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: request.RunID})
	if err != nil || current.Status != protocol.RunStatusRunning || current.Outcome != nil {
		t.Fatalf("terminal Run escaped its rolled-back receipt transaction: %+v, %v", current, err)
	}
	if hasConfirmedScopeRootCancellation(readWaitingChildScopeLog(t, home)) {
		t.Fatal("the Scope Exit escaped the refused receipt transaction")
	}
	if waitForTimeout {
		select {
		case original := <-result:
			if original.Failure == nil || !errors.Is(original.Failure, protocol.ErrInternalError) {
				t.Fatalf("the first request acknowledged an unsettled Run: %+v", original)
			}
		case <-time.After(lifecycleWaitBudget(t)):
			t.Fatal("the first request did not reach its cleanup deadline")
		}
		if pending := endpoint.Invoke(ctx, delivery.RunsCancel, request, options); pending.Failure == nil || !errors.Is(pending.Failure, protocol.ErrIdempotencyInProgress) {
			t.Fatalf("the first request replaced the pending operation with a reply: %+v", pending)
		}
	}
	questionCrashDatabase(t, home, "DROP TRIGGER refuse_live_cancel_receipt")
	if waitForTimeout {
		go func() {
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			for ctx.Err() == nil {
				reply := endpoint.Invoke(ctx, delivery.RunsCancel, request, options)
				if reply.Failure == nil || !errors.Is(reply.Failure, protocol.ErrIdempotencyInProgress) {
					result <- reply
					return
				}
				select {
				case <-ticker.C:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	select {
	case reply := <-result:
		if reply.Failure != nil {
			t.Fatal(reply.Failure)
		}
		original := reply.Value.(*protocol.CancelRunResponse)
		if original.Run.ID != request.RunID || original.Run.Outcome == nil ||
			original.Run.Outcome.Type != protocol.OutcomeCanceled || original.Run.Outcome.Detail != request.Reason {
			t.Fatalf("the original Run lost its cancellation after retry: %+v", original)
		}
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatal("the original cancellation did not settle after SQLite accepted its receipt")
	}
}

func startCancelableActiveRoot(t *testing.T, ctx context.Context, api *delivery.Handler, model *lifecycleModel, home string) protocol.CancelRunRequest {
	t.Helper()
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: home}, Title: "cancel a live root",
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
	_ = collectRunEvents(events)
	waitForSignal(t, model.firstCallStarted, "the active root's original model")
	return protocol.CancelRunRequest{RunID: started.RunID, Reason: activeRootCancelReason}
}
