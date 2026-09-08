package delivery

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/protocol"
)

// TestMapRunEvents_FramesWireEventID verifies delivery applies the evt_ wire
// framing to the stream position the application minted, and nothing
// else: the cursor's contents are the application's business, and this layer
// neither parses nor orders them.
func TestMapRunEvents_FramesWireEventID(t *testing.T) {
	cursors := []string{"AAAA", "BBBB", "CCCC"}
	in := slices.Values([]runs.Event{
		{RunID: "run_1", Cursor: cursors[0], Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}},
		{RunID: "run_1", Cursor: cursors[1], Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}},
		{RunID: "run_1", Cursor: cursors[2], Timestamp: time.Unix(0, 0), Payload: runs.SegmentProgressed{}},
	})

	var ids []string
	for e := range mapRunEvents(in) {
		if !strings.HasPrefix(e.EventID, "evt_") {
			t.Fatalf("eventId %q missing evt_ prefix", e.EventID)
		}
		ids = append(ids, e.EventID)
	}

	if len(ids) != len(cursors) {
		t.Fatalf("got %d events, want %d", len(ids), len(cursors))
	}
	for i, cursor := range cursors {
		if want := "evt_" + cursor; ids[i] != want {
			t.Fatalf("eventId[%d] = %q, want %q", i, ids[i], want)
		}
	}
}

func TestMapRunEvents_ReportsPresenterFailureAsTheStreamFailure(t *testing.T) {
	in := slices.Values([]runs.Event{
		{RunID: "run_1", Cursor: "AAAA"}, // nil payload is invalid
		{RunID: "run_1", Cursor: "BBBB", Payload: runs.SegmentProgressed{}},
	})

	var count int
	var failure error
	for _, err := range mapRunEvents(in) {
		if err != nil {
			failure = err
			break
		}
		count++
	}
	if count != 0 {
		t.Fatalf("events before the presenter failure = %d, want 0", count)
	}
	if !errors.Is(failure, protocol.ErrInternalError) {
		t.Fatalf("stream failure = %v, want an internal error", failure)
	}
}

func TestMapRunEvents_DoesNotRecoverConsumerPanic(t *testing.T) {
	in := slices.Values([]runs.Event{
		{RunID: "run_1", Cursor: "AAAA", Payload: runs.SegmentProgressed{}},
	})
	const want = "consumer panic"

	defer func() {
		if got := recover(); got != want {
			t.Fatalf("recovered panic = %v, want %q", got, want)
		}
	}()
	for range mapRunEvents(in) {
		panic(want)
	}
	t.Fatal("consumer panic was recovered by mapRunEvents")
}
