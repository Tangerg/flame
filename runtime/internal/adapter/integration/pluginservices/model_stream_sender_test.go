package pluginservices

import (
	"errors"
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestModelStreamSenderJoinsFailuresAfterReleasingTheSource(t *testing.T) {
	primary := errors.New("model failed")
	cleanup := errors.New("cleanup failed")
	var received []error
	var terminal *chat.ResponseDelta
	sender := newModelStreamSender(func(value *chat.ResponseDelta, err error) bool {
		received = append(received, err)
		terminal = value
		return true
	})
	if !sender.send(&chat.ResponseDelta{}, nil) {
		t.Fatal("open consumer lost a chunk")
	}
	partial := &chat.ResponseDelta{Metadata: &chat.ResponseMetadata{Model: "original"}}
	if sender.send(partial, primary) {
		t.Fatal("terminal source failure did not stop its producer")
	}
	partial.Metadata.Model = "reused"
	if len(received) != 1 || received[0] != nil {
		t.Fatalf("terminal failure preceded source release: %v", received)
	}
	sender.fail(cleanup)
	sender.finish()
	sender.finish()
	if len(received) != 2 || !errors.Is(received[1], primary) || !errors.Is(received[1], cleanup) {
		t.Fatalf("terminal outcome lost a cause or was repeated: %v", received)
	}
	if terminal == nil || terminal.Metadata.Model != "original" {
		t.Fatalf("terminal partial output changed after source release: %+v", terminal)
	}
	if sender.send(nil, nil) {
		t.Fatal("finished sender accepted another chunk")
	}
}

func TestModelStreamSenderNeverDeliversAfterTheConsumerStops(t *testing.T) {
	var received int
	sender := newModelStreamSender(func(*chat.ResponseDelta, error) bool {
		received++
		return false
	})
	if sender.send(&chat.ResponseDelta{}, nil) {
		t.Fatal("stopped consumer accepted another chunk")
	}
	sender.fail(errors.New("cleanup failed"))
	sender.finish()
	if received != 1 {
		t.Fatalf("stopped consumer received %d callbacks", received)
	}
}

func TestModelStreamSenderPreservesConsumerPanicDuringCleanup(t *testing.T) {
	panicCause := errors.New("consumer panic")
	var received int
	func() {
		defer func() {
			if recovered := recover(); recovered != panicCause {
				t.Fatalf("consumer panic changed: %v", recovered)
			}
		}()
		sender := newModelStreamSender(func(*chat.ResponseDelta, error) bool {
			received++
			panic(panicCause)
		})
		defer sender.finish()
		defer sender.fail(errors.New("cleanup failed"))
		sender.send(&chat.ResponseDelta{}, nil)
	}()
	if received != 1 {
		t.Fatalf("panicking consumer received %d callbacks", received)
	}
}
