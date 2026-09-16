package runs

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// panickingFinalizerEffects stands in for a defect anywhere under post-Run
// maintenance — the workspace checkpoint reaches the filesystem and a shell.
type panickingFinalizerEffects struct {
	*fakeEffects
}

func (panickingFinalizerEffects) Finish(context.Context, Finish) error {
	panic("segment: maintenance defect")
}

// The pump abandons a Run whose projection panicked and keeps the process. That
// bargain only holds if the finished Run still pays what it owes everyone else:
// its admission fences this Session's single-writer slot and working tree, and
// its journal is how a subscriber learns the Segment ended.
func TestTerminalMaintenanceDefectStillSettlesTheRunBoundary(t *testing.T) {
	sessions := &fakeRunSessions{
		sess: testsupport.MustRestoreSession(session.Snapshot{
			ID: "ses_1", Workspace: testsupport.MustWorkspace("/work"),
		}),
	}
	c := newUseCaseCoordinator(
		&fakeExecutor{events: []ExecutorPayload{SegmentEnded{Reason: run.OutcomeCompleted}}},
		&fakeExecutionPorts{startRef: ExecutorRef{SessionID: "ses_1", ExecutorID: "turn_1"}},
		sessions,
		panickingFinalizerEffects{fakeEffects: &fakeEffects{}},
	)

	result, err := c.Start(t.Context(), StartCommand{
		SessionID: "ses_1",
		Input:     []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	drained := make(chan struct{})
	go func() {
		defer close(drained)
		consumeEvents(result.Events)
	}()
	select {
	case <-drained:
	case <-time.After(10 * time.Second):
		t.Fatal("maintenance defect left the Run's stream open; no subscriber can leave it")
	}

	requireCoordinatorShutdown(t, c)
	if hasActiveSession(c, "ses_1") {
		t.Fatal("maintenance defect kept the admission fence; the Session can never admit another Run")
	}
	if release, ok, _ := c.admission.AcquireSession(t.Context(), "ses_1"); !ok {
		t.Fatal("Session stayed unadmittable after its Run settled")
	} else {
		release()
	}
}
