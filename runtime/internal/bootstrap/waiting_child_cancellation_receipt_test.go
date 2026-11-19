package bootstrap

import (
	"bytes"
	"context"
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
)

const (
	waitingChildReceiptEnv    = "FLAME_TEST_WAITING_CHILD_CANCEL_RECEIPT"
	waitingChildReceiptKey    = "original-waiting-child-cancel"
	waitingChildReceiptReason = "Cancel the originally interrupted sibling"
)

type waitingChildReceiptCrashStore struct {
	operation.ReplayStore
	path string
}

func (s waitingChildReceiptCrashStore) Complete(ctx context.Context, record operation.ReplayRecord) (operation.ReplayRecord, error) {
	original, found, err := s.ReplayStore.Lookup(ctx, record.Key, record.Fingerprint)
	if err != nil {
		return operation.ReplayRecord{}, err
	}
	if !found || len(original.Settlement) == 0 || !bytes.Equal(original.Settlement, record.Settlement) {
		return operation.ReplayRecord{}, errors.New("original waiting child cancellation settlement was not durably staged")
	}
	if err := os.WriteFile(s.path, record.Payload, 0o600); err != nil {
		return operation.ReplayRecord{}, err
	}
	os.Exit(commandCrashExit)
	return operation.ReplayRecord{}, nil
}

func TestWaitingChildCancellationReceiptCrashHelper(t *testing.T) {
	path := os.Getenv(waitingChildReceiptEnv)
	if path == "" {
		return
	}
	host, api, stores := openProtocolRuntimeWithStores(t, waitingSiblingModel())
	defer host.Close()
	meta := waitingChildCancellationMeta()
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	session, err := api.CreateSession(ctx, protocol.CreateSessionRequest{
		Workspace: &protocol.WorkspaceRef{Path: os.Getenv("FLAME_HOME")}, Title: "cancel waiting sibling",
	})
	if err != nil {
		t.Fatal(err)
	}
	opening, events, err := api.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate both siblings"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "first waiting sibling")
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: opening.RunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) == 0 {
		t.Fatalf("expected at least one waiting sibling: %+v, %v", pending, err)
	}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: waitingChildReceiptCrashStore{ReplayStore: stores.Idempotency, path: path},
		ValidateStorage:  stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := endpoint.Invoke(ctx, delivery.RunsCancel, protocol.CancelRunRequest{
		RunID: pending.Data[0].Interrupts[0].RunID, Reason: waitingChildReceiptReason,
	}, protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: waitingChildReceiptKey})
	t.Fatalf("waiting child cancellation did not reach its receipt cut: %+v", result.Failure)
}

func TestWaitingChildCancellationRecoversOriginalAcknowledgement(t *testing.T) {
	t.Setenv("FLAME_HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "original-child-cancel-outcome.json")
	crashCtx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	child := exec.CommandContext(crashCtx, os.Args[0], "-test.run=^TestWaitingChildCancellationReceiptCrashHelper$")
	child.Env = append(os.Environ(), waitingChildReceiptEnv+"="+path)
	output, err := child.CombinedOutput()
	if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != commandCrashExit {
		t.Fatalf("waiting child cancellation did not reach its receipt cut: %v\n%s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var original struct {
		Value *protocol.CancelRunResponse `json:"value"`
	}
	if err := json.Unmarshal(data, &original); err != nil {
		t.Fatal(err)
	}
	want := original.Value
	if want == nil || want.RootRun == nil || want.Type != protocol.CancelRunChild || want.Run.Outcome == nil ||
		want.Run.Outcome.Type != protocol.OutcomeCanceled || want.Run.Outcome.Detail != waitingChildReceiptReason {
		t.Fatalf("the original operation did not produce child cancellation: %s", data)
	}

	host, api := openProtocolRuntime(t, waitingSiblingModel())
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	meta := waitingChildCancellationMeta()
	ctx := delivery.WithRequestMeta(t.Context(), meta)
	answerWaitingSibling(t, ctx, api, want.RootRun.ID, want.Run.ID)
	finished, err := api.GetRun(ctx, protocol.GetRunRequest{RunID: want.RootRun.ID})
	if err != nil || finished.Outcome == nil || finished.Outcome.Type != protocol.OutcomeCompleted ||
		finished.Status == want.RootRun.Status {
		t.Fatalf("the parent did not move beyond the original result: %+v, %v; original %+v", finished, err, want.RootRun)
	}

	endpoint := host.Endpoint()
	request := protocol.CancelRunRequest{RunID: want.Run.ID, Reason: waitingChildReceiptReason}
	limited := protocol.CallMetadata{IdempotencyKey: waitingChildReceiptKey}
	if result := endpoint.Invoke(ctx, delivery.RunsCancel, request, limited); !errors.Is(result.Failure, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("unnegotiated child cancellation replay exposed its original outcome: %+v", result)
	}
	opts := protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: waitingChildReceiptKey}
	recovered, err := endpoint.Call[protocol.CancelRunRequest, *protocol.CancelRunResponse](ctx, delivery.RunsCancel, request, opts)
	if err != nil {
		t.Fatal(err)
	}
	got, expected := cancellationResponseJSON(t, recovered), cancellationResponseJSON(t, want)
	if !bytes.Equal(got, expected) {
		t.Fatalf("the replay adopted a later parent observation:\ngot  %s\nwant %s", got, expected)
	}
	if result := endpoint.Invoke(ctx, delivery.RunsCancel, request, limited); !errors.Is(result.Failure, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("settled child receipt bypassed capability negotiation: %+v", result)
	}
	request.Reason = "different reason"
	if result := endpoint.Invoke(ctx, delivery.RunsCancel, request, opts); !errors.Is(result.Failure, protocol.ErrIdempotencyConflict) {
		t.Fatalf("changed request reused the child cancellation key: %+v", result)
	}
}

func answerWaitingSibling(t *testing.T, ctx context.Context, api *delivery.Handler, rootRunID, canceledRunID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, lifecycleWaitBudget(t))
	defer cancel()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var survivor protocol.Interrupt
	for survivor.ItemID == "" {
		pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: rootRunID})
		if err != nil {
			t.Fatal(err)
		}
		if len(pending.Data) == 1 && len(pending.Data[0].Interrupts) == 1 {
			survivor = pending.Data[0].Interrupts[0]
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("the surviving sibling did not retain its question")
		}
	}
	if survivor.RunID == canceledRunID {
		t.Fatalf("the canceled sibling %q published another question", canceledRunID)
	}
	_, events, err := api.ResumeRun(ctx, protocol.ResumeRunRequest{
		RunID: rootRunID, Responses: []protocol.InterruptResponse{{
			ItemID:   survivor.ItemID,
			Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"Yes"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForRunEvents(t, collectRunEvents(events), "answered surviving sibling")
	waitForProtocolRunTerminal(t, ctx, api, rootRunID)
}
