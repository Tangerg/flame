package http

import (
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"

	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
)

// The bridge goroutine is what drives the frame source, so the handler's own
// recovery cannot see a defect raised there. One client's stream is not worth
// the process: it ends the way the dispatch already ends a stream whose event it
// cannot deliver, and the client replays the gap with Last-Event-Id.
func TestStreamSourceDefectEndsOneStreamNotTheProcess(t *testing.T) {
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })

	id, err := jsonrpc.MakeID("1")
	if err != nil {
		t.Fatalf("make request id: %v", err)
	}
	ack, err := transport.NewResponseResult(id, map[string]string{"runId": "run_x"})
	if err != nil {
		t.Fatalf("build ack: %v", err)
	}
	first, err := transport.NewNotification("notifications.run.event", map[string]string{"runId": "run_x"})
	if err != nil {
		t.Fatalf("build notification: %v", err)
	}

	events := func(yield func(dispatch.StreamFrame) bool) {
		if !yield(dispatch.StreamFrame{Notification: first, SSEID: "evt_00000000001"}) {
			return
		}
		panic("dispatch: run event projection defect")
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(stdhttp.MethodPost, "/v2/rpc", nil)
	served := make(chan struct{})
	go func() {
		defer close(served)
		(&Server{}).serveStream(recorder, request, ack, events, "runs.start")
	}()
	select {
	case <-served:
	case <-time.After(10 * time.Second):
		t.Fatal("stream never ended after its source panicked")
	}

	frames := parseSSEFrames(recorder.Body.String())
	if len(frames) != 2 {
		t.Fatalf("frames written = %d, want the ack and the one frame the source produced: %q",
			len(frames), recorder.Body.String())
	}
	if frames[1] != "evt_00000000001" {
		t.Fatalf("second frame SSE id = %q, want the delivered event", frames[1])
	}
}

// parseSSEFrames lifts one id per data frame; the ack carries none.
func parseSSEFrames(raw string) []string {
	var ids []string
	for block := range strings.SplitSeq(strings.TrimSpace(raw), "\n\n") {
		id, hasData := "", false
		for line := range strings.SplitSeq(block, "\n") {
			switch {
			case strings.HasPrefix(line, "id:"):
				id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			case strings.HasPrefix(line, "data:"):
				hasData = true
			}
		}
		if hasData {
			ids = append(ids, id)
		}
	}
	return ids
}
