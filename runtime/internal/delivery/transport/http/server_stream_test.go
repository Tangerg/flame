package http_test

import (
	"bytes"
	"context"
	netHTTP "net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func (f *fakeRuns) Start(_ context.Context, in runs.StartCommand) (runs.StartResult, error) {
	started := testsupport.MustRestoreRun(run.Snapshot{ID: "run_x", SessionID: in.SessionID, State: run.Running, ActiveSegmentID: "seg_x", CreatedAt: time.Unix(1, 0).UTC()})
	finished := testsupport.MustRestoreRun(run.Snapshot{ID: "run_x", SessionID: in.SessionID, State: run.Completed, CreatedAt: time.Unix(1, 0).UTC(), FinishedAt: time.Unix(2, 0).UTC()})
	events := slices.Values([]runs.Event{
		{RunID: "run_x", SegmentID: "seg_x", Cursor: "00000000001", Timestamp: time.Unix(1, 0).UTC(), Payload: runs.SegmentStarted{Run: started}},
		{RunID: "run_x", SegmentID: "seg_x", Cursor: "00000000002", Timestamp: time.Unix(2, 0).UTC(), Payload: runs.SegmentFinished{Run: finished}},
	})
	return runs.StartResult{RunID: "run_x", SegmentID: "seg_x", SessionID: in.SessionID, UserItemID: "item_x", Events: events}, nil
}

type sseFrame struct{ id, data string }

// parseSSE splits a text/event-stream body into frames, lifting the id and
// data lines and skipping comments / blanks.
func parseSSE(raw string) []sseFrame {
	var out []sseFrame
	for _, block := range strings.Split(strings.TrimSpace(raw), "\n\n") {
		var f sseFrame
		hasData := false
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "id:"):
				f.id = strings.TrimSpace(line[len("id:"):])
			case strings.HasPrefix(line, "data:"):
				f.data += strings.TrimSpace(line[len("data:"):])
				hasData = true
			}
		}
		if hasData {
			out = append(out, f)
		}
	}
	return out
}

// TestStreamableRunStart confirms a streaming method's POST response is itself
// the event stream: 200 text/event-stream, first frame is the JSON-RPC ack, then
// run-event frames each carrying eventId as the SSE id.
func TestStreamableRunStart(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r0, _ := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
	_ = r0.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.start","params":{"sessionId":"ses_1","input":[{"type":"text","text":"hi"}]}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != netHTTP.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	frames := parseSSE(readBody(resp))
	if len(frames) != 3 {
		t.Fatalf("frames = %d, want 3 (ack + started + finished)", len(frames))
	}
	if frames[0].id != "" || !strings.Contains(frames[0].data, `"runId":"run_x"`) {
		t.Fatalf("ack frame = %+v, want runId result with no SSE id", frames[0])
	}
	if frames[1].id != "evt_00000000001" || !strings.Contains(frames[1].data, "segment.started") {
		t.Fatalf("frame[1] = %+v, want segment.started @ evt 1", frames[1])
	}
	if frames[2].id != "evt_00000000002" || !strings.Contains(frames[2].data, "segment.finished") {
		t.Fatalf("frame[2] = %+v, want segment.finished @ evt 2", frames[2])
	}
}

// Subscribe observes the cursor after Handler removes its wire framing.
func (f *fakeRuns) Subscribe(_ context.Context, in runs.SubscribeRequest) (runs.Subscription, error) {
	f.gotLastEventID = in.Cursor
	return runs.Subscription{Record: runs.Record{ID: in.RunID, SegmentID: in.SegmentID}, Events: slices.Values([]runs.Event{})}, nil
}

// TestSubscribeCarriesLastEventID confirms the transport lifts the
// Last-Event-Id request header onto the ctx so runs.subscribe resumes from it
// instead of full-replaying.
func TestSubscribeCarriesLastEventID(t *testing.T) {
	ts, api := newTestServer(t)
	defer ts.Close()

	discoverBody := []byte(`{"jsonrpc":"2.0","id":"1","method":"runtime.discover","params":{}}`)
	r0, _ := netHTTP.Post(ts.URL+"/v2/rpc", "application/json", bytes.NewReader(discoverBody))
	_ = r0.Body.Close()

	body := []byte(`{"jsonrpc":"2.0","id":"2","method":"runs.subscribe","params":{"runId":"run_1","segmentId":"seg_1"}}`)
	req, _ := netHTTP.NewRequest("POST", ts.URL+"/v2/rpc", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Last-Event-Id", "evt_00000000042")
	resp, err := netHTTP.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()

	if api.gotLastEventID != "00000000042" {
		t.Fatalf("SubscribeRun saw Last-Event-Id %q, want 00000000042", api.gotLastEventID)
	}
}
