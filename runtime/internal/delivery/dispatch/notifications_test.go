package dispatch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestEncodeRuntimeEventRejectsAnInvalidOutputShape(t *testing.T) {
	t.Parallel()

	_, err := EncodeRuntimeEvent(protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Sequence: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "RuntimeEvent.topics") {
		t.Fatalf("EncodeRuntimeEvent error = %v, want shape-qualified topics violation", err)
	}
}

func TestDispatchNotificationSuppressesMetadataErrors(t *testing.T) {
	router := &Router{}
	message := &transport.Request{
		Method: "client.unknown",
		Params: json.RawMessage(`{"_meta":null}`),
	}

	if got := router.Dispatch(context.Background(), message); got.Response != nil {
		t.Fatalf("notification returned a response: %+v", got.Response)
	}
}

func TestEphemeralEventNeverCarriesAnSSEReplayID(t *testing.T) {
	t.Parallel()

	frame, ok := runEventToFrame(protocol.RunEvent{
		RunID: "run_1", SegmentID: "seg_1", EventID: "evt_1",
		Timestamp: time.Unix(1, 0).UTC(),
		Event: protocol.StreamEvent{
			Type:     protocol.StreamSegmentProgress,
			Progress: &protocol.RunProgress{Activity: "Calling model"},
		},
	})
	if !ok {
		t.Fatal("segment progress event was not encoded")
	}
	if frame.SSEID != "" {
		t.Fatalf("ephemeral SSE id = %q, want none", frame.SSEID)
	}
}

// TestStreamEndsOnAnUnpublishableEvent pins the difference between a stream that
// ended and a stream with a hole in it: a frame the transport cannot encode
// stops delivery, so the client resubscribes and replays instead of believing
// it saw everything.
func TestStreamEndsOnAnUnpublishableEvent(t *testing.T) {
	t.Parallel()

	events := func(yield func(any, error) bool) {
		if !yield(protocol.RuntimeEvent{Type: protocol.RuntimeSkillsChanged, Sequence: 1}, nil) {
			return
		}
		if !yield("not a runtime event", nil) {
			return
		}
		yield(protocol.RuntimeEvent{Type: protocol.RuntimeSessionsChanged, Sequence: 2}, nil)
	}

	var frames int
	for range adaptOperationEvents(events) {
		frames++
	}
	if frames != 1 {
		t.Fatalf("frames = %d, want only the events published before the unpublishable one", frames)
	}
}
