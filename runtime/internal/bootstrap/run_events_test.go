package bootstrap

import (
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
