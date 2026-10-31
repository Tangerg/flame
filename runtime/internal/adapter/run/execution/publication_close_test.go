package execution

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

func TestFinishWithdrawsAndJoinsEventSendersBeforeClosingPublication(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		session := &interactionSession{lifetime: newInteractionLifetime(t.Context())}
		for session.lifetime.offer(runs.ExecutorEvent{}) {
		}
		sent := make(chan bool, 1)
		committed := make(chan error, 1)
		requested := make(chan error, 1)
		go func() { sent <- session.lifetime.send(runs.ExecutorEvent{}) }()
		go func() { committed <- session.lifetime.sendAuthoritative(t.Context(), runs.ExecutorEvent{}) }()
		go func() { requested <- session.sendExecutorRequest(t.Context(), runs.ExecutorEvent{}) }()
		synctest.Wait()
		session.finish()
		synctest.Wait()
		inspection, releaseInspection := session.lifetime.publicationContext(t.Context())
		defer releaseInspection()
		if err := inspection.Err(); err != nil {
			t.Fatalf("finishing event publication canceled retained terminal inspection: %v", err)
		}
		if <-sent || !errors.Is(<-committed, errInteractionReleased) {
			t.Fatal("withdrawn senders published into a finished session")
		}
		if err := <-requested; !errors.Is(err, errInteractionReleased) && !errors.Is(err, context.Canceled) {
			t.Fatalf("withdrawn executor request=%v", err)
		}
		if session.lifetime.offer(runs.ExecutorEvent{}) || session.lifetime.send(runs.ExecutorEvent{}) {
			t.Fatal("late observation published after channel closure")
		}
		if err := session.lifetime.sendAuthoritative(t.Context(), runs.ExecutorEvent{}); !errors.Is(err, errInteractionReleased) {
			t.Fatalf("late authoritative publication=%v", err)
		}
		if err := session.sendExecutorRequest(t.Context(), runs.ExecutorEvent{}); !errors.Is(err, errInteractionReleased) {
			t.Fatalf("late executor request=%v", err)
		}
		buffered := 0
		for range session.lifetime.events {
			buffered++
		}
		if buffered != interactionEventBuffer {
			t.Fatalf("finish lost buffered publication: %d", buffered)
		}
	})
}
