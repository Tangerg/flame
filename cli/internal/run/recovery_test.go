package run_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/cli/internal/agent"
	runworkflow "github.com/Tangerg/flame/cli/internal/run"
	"github.com/Tangerg/flame/cli/internal/testsupport/runtimefixture"
)

func TestRecoverReadsAFinishedRunAfterItsSegmentExpires(t *testing.T) {
	runtime := runtimefixture.New()
	runtime.Instant = true
	runtime.Script = completedScript
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStart(session.ID, "finish"))
	if err != nil {
		t.Fatal(err)
	}
	consumeSegment(t, opened)
	if _, subscribeRunErr := runtime.SubscribeRun(t.Context(), agent.SubscribeRun{RunID: opened.RunID, SegmentID: opened.SegmentID}); !runworkflow.RecoveryRequired(subscribeRunErr) {
		t.Fatalf("subscribe error = %v, want a cold-recovery condition", subscribeRunErr)
	}
	recovered, err := runworkflow.RecoverSegment(t.Context(), runtime, session.ID, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.Status != agent.RunStatusFinished || recovered.Run.Outcome.Status != agent.OutcomeCompleted || recovered.Stream.Events != nil {
		t.Fatalf("recovered state = %+v", recovered)
	}
	if len(recovered.Snapshot.Transcript) != 2 {
		t.Fatalf("transcript = %+v, want user and assistant blocks", recovered.Snapshot.Transcript)
	}
}

func TestRecoverAttachesBeforeReadingALiveRun(t *testing.T) {
	runtime := runtimefixture.New()
	runtime.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStart(session.ID, "keep running"))
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := runworkflow.RecoverSegment(t.Context(), runtime, session.ID, opened.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.Status != agent.RunStatusRunning || recovered.Stream.RunID != opened.RunID || recovered.Stream.SegmentID != opened.SegmentID || recovered.Stream.Events == nil {
		t.Fatalf("recovered state = %+v", recovered)
	}
	conversation := agent.NewConversation()
	if err := conversation.RestoreAttachedSnapshot(recovered.Snapshot, recovered.Stream); err != nil {
		t.Fatal(err)
	}
	if conversation.Checkpoint() != recovered.Stream.HeadEventID || conversation.Checkpoint() == "" {
		t.Fatalf("recovery checkpoint = %q, head = %q", conversation.Checkpoint(), recovered.Stream.HeadEventID)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID, Reason: "test complete"}); err != nil {
		t.Fatal(err)
	}
}

func TestAttachSessionPerformsTheHeadAttachmentBeforeItsAuthoritativeRead(t *testing.T) {
	runtime := runtimefixture.New()
	runtime.Script = func(string) runtimefixture.Script {
		return runtimefixture.Script{Prelude: []runtimefixture.Step{{Delay: time.Hour, Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}}}}
	}
	session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := runtime.StartRun(t.Context(), unlimitedStart(session.ID, "keep running"))
	if err != nil {
		t.Fatal(err)
	}
	observed := &orderedSource{source: runtime}
	recovered, err := runworkflow.AttachSession(t.Context(), observed, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Run.ID != opened.RunID || recovered.Stream.Events == nil {
		t.Fatalf("attached state = %+v", recovered)
	}
	if got := observed.snapshot(); !slices.Equal(got, []string{"read", "attach", "read"}) {
		t.Fatalf("recovery operations = %v", got)
	}
	if _, err := runtime.CancelRun(t.Context(), agent.CancelRun{RunID: opened.RunID, Reason: "test complete"}); err != nil {
		t.Fatal(err)
	}
}

func TestAttachSessionReturnsAuthoritativeStateWhenNoStreamIsRequired(t *testing.T) {
	t.Run("waiting", func(t *testing.T) {
		runtime := runtimefixture.New()
		runtime.Instant = true
		runtime.Script = func(string) runtimefixture.Script {
			return runtimefixture.Script{Interactions: []agent.Interaction{agent.Approval{
				ItemID: "approval_1", Title: "Run checks",
				Tool: &agent.ToolCall{Kind: agent.ToolShell, Name: "shell", Command: "go test ./...", Status: agent.ToolRunning},
			}}}
		}
		session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		opened, err := runtime.StartRun(t.Context(), agent.StartRun{
			SessionID: session.ID, Message: agent.Message{Text: "wait for approval"},
			Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
		})
		if err != nil {
			t.Fatal(err)
		}
		consumeSegment(t, opened)

		recovered, err := runworkflow.AttachSession(t.Context(), runtime, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Run.ID != opened.RunID || recovered.Run.Status != agent.RunStatusWaiting ||
			recovered.Stream.Events != nil || len(recovered.Snapshot.Interactions) != 1 {
			t.Fatalf("waiting session attachment = %+v", recovered)
		}
	})

	t.Run("finished", func(t *testing.T) {
		runtime := runtimefixture.New()
		runtime.Instant = true
		runtime.Script = completedScript
		session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		opened, err := runtime.StartRun(t.Context(), agent.StartRun{
			SessionID: session.ID, Message: agent.Message{Text: "finish"},
			Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
		})
		if err != nil {
			t.Fatal(err)
		}
		consumeSegment(t, opened)

		recovered, err := runworkflow.AttachSession(t.Context(), runtime, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Run.ID != opened.RunID || recovered.Run.Status != agent.RunStatusFinished || recovered.Stream.Events != nil {
			t.Fatalf("finished session attachment = %+v", recovered)
		}
	})

	t.Run("empty", func(t *testing.T) {
		runtime := runtimefixture.New()
		session, err := runtime.CreateSession(t.Context(), agent.CreateSession{Workspace: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		recovered, err := runworkflow.AttachSession(t.Context(), runtime, session.ID)
		if err != nil {
			t.Fatal(err)
		}
		if recovered.Run.ID != "" || recovered.Stream.Events != nil || len(recovered.Snapshot.Runs) != 0 {
			t.Fatalf("empty session attachment = %+v", recovered)
		}
	})
}

func unlimitedStart(sessionID, text string) agent.StartRun {
	return agent.StartRun{
		SessionID: sessionID, Message: agent.Message{Text: text},
		Options: agent.RunOptions{Limits: agent.UnlimitedRunLimits()},
	}
}

func consumeSegment(t *testing.T, stream agent.SegmentStream) {
	t.Helper()
	for _, streamErr := range stream.Events {
		if streamErr != nil {
			t.Fatal(streamErr)
		}
	}
}

func TestRequiredRecognizesOnlyColdRecoveryConditions(t *testing.T) {
	for _, err := range []error{
		agent.ErrStaleSegment, agent.ErrRunWaiting, agent.ErrRunFinished,
		agent.ErrReplayCursorInvalid, agent.ErrReplayUnavailable,
	} {
		if !runworkflow.RecoveryRequired(errors.Join(errors.New("adapter"), err)) {
			t.Fatalf("RecoveryRequired(%v) = false", err)
		}
	}
	if runworkflow.RecoveryRequired(agent.ErrDisconnected) {
		t.Fatal("a transport disconnect was classified as cold recovery")
	}
}

func completedScript(string) runtimefixture.Script {
	return runtimefixture.Script{Prelude: []runtimefixture.Step{
		{Event: agent.BlockCompleted{Block: agent.Block{ID: "answer", Kind: agent.BlockAssistant, Text: "done"}}},
		{Event: agent.RunFinished{Outcome: agent.Outcome{Status: agent.OutcomeCompleted}}},
	}}
}

type orderedSource struct {
	source runworkflow.RecoverySource

	mu         sync.Mutex
	operations []string
}

func (o *orderedSource) GetSession(ctx context.Context, id string) (agent.SessionSnapshot, error) {
	o.record("read")
	return o.source.GetSession(ctx, id)
}

func (o *orderedSource) SubscribeRun(ctx context.Context, request agent.SubscribeRun) (agent.SegmentStream, error) {
	o.record("attach")
	return o.source.SubscribeRun(ctx, request)
}

func (o *orderedSource) record(operation string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.operations = append(o.operations, operation)
}

func (o *orderedSource) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.operations)
}
