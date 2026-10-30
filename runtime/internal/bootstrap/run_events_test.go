package bootstrap

import (
	"context"
	"errors"
	"iter"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type runEventCollection struct {
	events []protocol.RunEvent
	err    error
}

func collectRunEvents(events iter.Seq2[protocol.RunEvent, error]) <-chan runEventCollection {
	done := make(chan runEventCollection, 1)
	go func() {
		defer close(done)
		var result runEventCollection
		for event, err := range events {
			if err != nil {
				result.err = err
				break
			}
			result.events = append(result.events, event)
		}
		done <- result
	}()
	return done
}

func waitForRunEvents(t *testing.T, done <-chan runEventCollection, phase string) []protocol.RunEvent {
	t.Helper()
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("%s event stream failed: %v", phase, result.err)
		}
		if len(result.events) == 0 {
			t.Fatalf("%s emitted no run events", phase)
		}
		return result.events
	case <-time.After(lifecycleWaitBudget(t)):
		t.Fatalf("timed out waiting for %s events", phase)
		return nil
	}
}

func TestRunEventCollectionDistinguishesEOFAndOriginalErrors(t *testing.T) {
	failure := errors.New("projection disconnected")
	cause := errors.New("consumer withdrew")
	for _, terminal := range []error{nil, failure, errors.Join(context.Canceled, cause)} {
		stream := func(yield func(protocol.RunEvent, error) bool) {
			if !yield(protocol.RunEvent{RunID: "run_prefix"}, nil) {
				return
			}
			if terminal != nil {
				yield(protocol.RunEvent{}, terminal)
			}
		}
		result := <-collectRunEvents(stream)
		if len(result.events) != 1 || result.events[0].RunID != "run_prefix" || !errors.Is(result.err, terminal) {
			t.Fatalf("collection = %+v, want original prefix and %v", result, terminal)
		}
		if errors.Is(terminal, cause) && (!errors.Is(result.err, context.Canceled) || !errors.Is(result.err, cause)) {
			t.Fatalf("collection lost cancellation identity: %v", result.err)
		}
	}
}

func TestRunEventCollectionKeepsBusinessFailureAsAnEvent(t *testing.T) {
	stream := func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{Event: protocol.StreamEvent{
			Type: protocol.StreamSegmentFinished,
			Run:  &protocol.RunRef{Outcome: &protocol.RunOutcome{Type: protocol.OutcomeFailed}},
		}}, nil)
	}
	events := waitForRunEvents(t, collectRunEvents(stream), "expected business failure")
	if len(events) != 1 || events[0].Event.Run.Outcome.Type != protocol.OutcomeFailed {
		t.Fatalf("lost business failure event: %+v", events)
	}
}
