package runs

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type capturedMessages struct {
	slog.Handler
	mu       sync.Mutex
	messages []string
}

func (c *capturedMessages) Handle(_ context.Context, record slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, record.Message)
	return nil
}

func (c *capturedMessages) count(substring string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	matches := 0
	for _, message := range c.messages {
		if strings.Contains(message, substring) {
			matches++
		}
	}
	return matches
}

// The terminal write for an Effect whose durable outcome is unknown is the one
// commit that may not be abandoned, so it retries without a bound while the Run
// stays blocked. A span reaches nobody on a host that configured no tracing;
// the operator needs one report per outage, not one per attempt and not none.
func TestUnknownEffectLossOutageReachesTheOperatorOnce(t *testing.T) {
	outage := errors.New("terminal write-set store unavailable")
	executor := &fakeExecutor{executorEvents: []ExecutorEvent{
		{
			Member:  ExecutorMember{MemberID: "member_root"},
			Payload: NewSegmentEnded(run.OutcomeLost, &run.Failure{Kind: run.FailureLost}, nil, 0),
		},
	}}
	effects := &fakeEffects{commitErr: outage, commitErrAt: 1, commitErrCount: 3}
	coordinator := testCoordinator(executor, effects)

	captured := &capturedMessages{Handler: slog.Default().Handler()}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(captured))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	stream, err := coordinator.openSegment(t.Context(), testSegment())
	if err != nil {
		t.Fatalf("openSegment: %v", err)
	}
	events := collectEvents(stream)
	requireCoordinatorShutdown(t, coordinator)

	if len(events) == 0 {
		t.Fatal("Run stream is empty")
	}
	finished, ok := events[len(events)-1].Payload.(SegmentFinished)
	if !ok || !runHasOutcome(finished.Run, run.OutcomeLost) {
		t.Fatalf("last event = %#v, want a lost SegmentFinished after the outage cleared", events[len(events)-1].Payload)
	}
	if reports := captured.count("terminal commit failed"); reports != 1 {
		t.Fatalf("operator reports = %d, want exactly one for one outage spanning several retries", reports)
	}
}
