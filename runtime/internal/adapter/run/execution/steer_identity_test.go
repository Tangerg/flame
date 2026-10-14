package execution

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/agent"
)

type heldSteerAdmission struct {
	runs.ExecutionTreeStore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (s *heldSteerAdmission) SaveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	tree, err := agent.ParseTreeSnapshot(update.Head.Payload)
	if err != nil {
		return err
	}
	for _, process := range tree.ProcessSnapshots() {
		for _, receipt := range process.SignalReceipts() {
			if receipt.ID().String() == "steer:item_original" {
				s.once.Do(func() { close(s.entered); <-s.release })
			}
		}
	}
	return s.ExecutionTreeStore.SaveExecutionTree(ctx, update)
}

func TestOriginalSteerSurvivesCallerCancellationWithoutASecondSignal(t *testing.T) {
	model := &runtimeSteerModel{started: make(chan struct{}), release: make(chan struct{})}
	var modelRelease sync.Once
	t.Cleanup(func() { modelRelease.Do(func() { close(model.release) }) })
	store := &heldSteerAdmission{ExecutionTreeStore: testTrees(t), entered: make(chan struct{}), release: make(chan struct{})}
	var admissionRelease sync.Once
	t.Cleanup(func() { admissionRelease.Do(func() { close(store.release) }) })
	executor := newObservedTestInteractionExecutor(t, model, InteractionExecutorConfig{ExecutionTrees: store})
	start := interactionTestStart()
	ref, err := executor.StageRoot(t.Context(), start)
	if err != nil {
		t.Fatal(err)
	}
	sequence, err := observeTestInteraction(t, executor, t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	events := collectInteractionEvents(sequence)
	if err := executor.BeginRoot(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	<-model.started
	input := []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "apply once"}}
	prepared := runs.PreparedSteer{ItemID: "item_original", Input: input, Acceptance: &runs.CommandAcceptance{
		Key: "original-key", Fingerprint: strings.Repeat("a", 64), Operation: runs.CommandSteerInput, SessionID: start.SessionID,
		RunID: "run_original", SegmentID: "seg_original", UserItemID: "item_original", SteerInput: input,
	}}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result := make(chan error, 1)
	go func() { _, err := executor.SubmitSteer(ctx, ref, prepared); result <- err }()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("steer admission never reached its commit")
	}
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation=%v", err)
	}
	admissionRelease.Do(func() { close(store.release) })
	retryCtx, retryCancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer retryCancel()
	itemID, err := executor.SubmitSteer(retryCtx, ref, prepared)
	if err != nil || itemID != prepared.ItemID {
		t.Fatalf("same original signal: %q,%v", itemID, err)
	}
	conflicting := prepared.Clone()
	conflicting.Acceptance.Fingerprint = strings.Repeat("b", 64)
	if _, err := executor.SubmitSteer(t.Context(), ref, conflicting); !errors.Is(err, runs.ErrCommandKeyConflict) {
		t.Fatalf("conflicting pending command=%v", err)
	}
	modelRelease.Do(func() { close(model.release) })
	observed := <-events
	applied := payloadsOf[runs.SteerMessagesApplied](observed)
	if len(applied) != 1 || len(applied[0].Messages) != 1 || applied[0].Messages[0].ItemID != prepared.ItemID {
		t.Fatalf("original command applied twice: %+v", applied)
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
}
