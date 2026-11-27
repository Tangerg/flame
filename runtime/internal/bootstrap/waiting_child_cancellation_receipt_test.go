package bootstrap

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"iter"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
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
	path          string
	awaitSurvivor func() protocol.Interrupt
}

type waitingChildReceiptCut struct {
	Outcome         jsontext.Value `json:"outcome"`
	SurvivingRunID  string         `json:"survivingRunId"`
	SurvivingItemID string         `json:"survivingItemId"`
}

func (s waitingChildReceiptCrashStore) Complete(ctx context.Context, record operation.ReplayRecord) (operation.ReplayRecord, error) {
	if len(record.Settlement) == 0 {
		return s.ReplayStore.Complete(ctx, record)
	}
	original, found, err := s.ReplayStore.Lookup(ctx, record.Key, record.Fingerprint)
	if err != nil {
		return operation.ReplayRecord{}, err
	}
	if !found || len(original.Settlement) == 0 || !bytes.Equal(original.Settlement, record.Settlement) {
		return operation.ReplayRecord{}, errors.New("original waiting child cancellation settlement was not durably staged")
	}
	// A released sibling can enter an Effect outside the cancellation
	// recovery window. Cut only once its question is durably waiting.
	survivor := s.awaitSurvivor()
	data, err := json.Marshal(waitingChildReceiptCut{
		Outcome: record.Payload, SurvivingRunID: survivor.RunID, SurvivingItemID: survivor.ItemID,
	})
	if err != nil {
		return operation.ReplayRecord{}, err
	}
	if err := os.WriteFile(s.path, data, 0o600); err != nil {
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
	canceledRunID := pending.Data[0].Interrupts[0].RunID
	watching, stopWatching := context.WithTimeout(ctx, lifecycleWaitBudget(t))
	defer stopWatching()
	_, changes, err := api.SubscribeRuntime(watching, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicInterruptsChanged},
	})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyNamespace: stores.IdempotencyNamespace.String(),
		IdempotencyStore: waitingChildReceiptCrashStore{
			ReplayStore: stores.Idempotency, path: path,
			awaitSurvivor: func() protocol.Interrupt {
				return awaitSurvivingSibling(t, watching, api, opening.RunID, canceledRunID, changes)
			},
		},
		ValidateStorage: stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	result := endpoint.Invoke(ctx, delivery.RunsCancel, protocol.CancelRunRequest{
		RunID: canceledRunID, Reason: waitingChildReceiptReason,
	}, protocol.CallMetadata{RequestMeta: meta, IdempotencyKey: waitingChildReceiptKey})
	if result.Failure == nil {
		t.Fatal("waiting child cancellation returned without reaching its receipt cut")
	}
	if settlement, ok := errors.AsType[*runs.CancellationSettlementError](result.Failure); ok {
		t.Fatalf("waiting child cancellation did not reach its receipt cut: %v", settlement)
	}
	t.Fatalf("waiting child cancellation did not reach its receipt cut: %v (%v)", result.Failure, errors.Unwrap(result.Failure))
}

func awaitSurvivingSibling(t *testing.T, ctx context.Context, api *delivery.Handler, rootRunID, canceledRunID string, changes iter.Seq2[protocol.RuntimeEvent, error]) protocol.Interrupt {
	t.Helper()
	next, stop := iter.Pull2(changes)
	defer stop()
	for {
		pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: rootRunID})
		if err != nil {
			t.Fatal(err)
		}
		if len(pending.Data) == 1 && len(pending.Data[0].Interrupts) == 1 {
			survivor := pending.Data[0].Interrupts[0]
			if survivor.RunID != canceledRunID && survivor.RunID != rootRunID {
				return survivor
			}
		}
		_, err, ok := next()
		if err != nil || !ok {
			t.Fatalf("the surviving sibling did not commit its question before the receipt cut: %+v, %v, %v", pending, err, ctx.Err())
		}
	}
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
	var cut waitingChildReceiptCut
	if err := json.Unmarshal(data, &cut); err != nil {
		t.Fatal(err)
	}
	var original struct {
		Value *protocol.CancelRunResponse `json:"value"`
	}
	if err := json.Unmarshal(cut.Outcome, &original); err != nil {
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
	answerWaitingSibling(t, ctx, api, want.RootRun.ID, cut)
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

func answerWaitingSibling(t *testing.T, ctx context.Context, api *delivery.Handler, rootRunID string, cut waitingChildReceiptCut) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, lifecycleWaitBudget(t))
	defer cancel()
	pending, err := api.ListInterrupts(ctx, protocol.ListInterruptsRequest{RootRunID: rootRunID})
	if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
		t.Fatalf("the surviving sibling did not retain its question %q: %+v, %v", cut.SurvivingItemID, pending, err)
	}
	survivor := pending.Data[0].Interrupts[0]
	if survivor.RunID != cut.SurvivingRunID || survivor.ItemID != cut.SurvivingItemID {
		t.Fatalf("the surviving question changed across restart: %+v, want %s / %s", survivor, cut.SurvivingRunID, cut.SurvivingItemID)
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
