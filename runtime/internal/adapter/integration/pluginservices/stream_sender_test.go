package pluginservices

import (
	"errors"
	"testing"
)

func TestStreamSenderNeverDeliversAfterTheConsumerStops(t *testing.T) {
	var received []error
	sender := newStreamSender(func(_ *int, err error) bool {
		received = append(received, err)
		return false
	})
	first := errors.New("unsupported stream")
	sender.fail(first)
	sender.fail(errors.New("cleanup failed"))
	if sender.send(nil, nil) {
		t.Fatal("a stopped sender reported a delivered item")
	}
	if len(received) != 1 || !errors.Is(received[0], first) {
		t.Fatalf("received = %v, want only the first failure", received)
	}
}

func TestStreamSenderDeliversCleanupFailureToAListeningConsumer(t *testing.T) {
	var received []error
	sender := newStreamSender(func(_ *int, err error) bool {
		received = append(received, err)
		return true
	})
	sender.fail(errors.New("unsupported stream"))
	sender.fail(errors.New("cleanup failed"))
	if len(received) != 2 {
		t.Fatalf("received %d failures, want both while the consumer listens", len(received))
	}
}
