package http

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/go-sdk/jsonrpc"

	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	transport "github.com/Tangerg/flame/runtime/protocol/jsonrpc"
)

type blockedChunkWriter struct {
	*httptest.ResponseRecorder
	entered chan struct{}
	release chan struct{}
}

func (w *blockedChunkWriter) Write(value []byte) (int, error) {
	if bytes.Contains(value, []byte("notifications.plugin.stream")) {
		close(w.entered)
		<-w.release
	}
	return w.ResponseRecorder.Write(value)
}

func TestStreamProductionWaitsForChunkWrite(t *testing.T) {
	id, err := jsonrpc.MakeID("original")
	if err != nil {
		t.Fatal(err)
	}
	ack, err := transport.NewResponseResult(id, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	chunk, err := transport.NewNotification("notifications.plugin.stream", map[string]string{"kind": "chunk", "value": "partial"})
	if err != nil {
		t.Fatal(err)
	}
	writer := &blockedChunkWriter{ResponseRecorder: httptest.NewRecorder(), entered: make(chan struct{}), release: make(chan struct{})}
	advanced := make(chan struct{})
	events := func(yield func(dispatch.StreamFrame) bool) {
		if yield(dispatch.StreamFrame{Notification: chunk}) {
			close(advanced)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request := httptest.NewRequest(stdhttp.MethodPost, "/v2/rpc", nil).WithContext(ctx)
	served := make(chan struct{})
	go func() {
		defer close(served)
		serveStream(writer, request, ack, events, "plugins.stream", cancel)
	}()
	select {
	case <-writer.entered:
	case <-time.After(time.Second):
		close(writer.release)
		t.Fatal("chunk write did not begin")
	}
	select {
	case <-advanced:
		close(writer.release)
		t.Fatal("production advanced before the chunk was written")
	case <-time.After(50 * time.Millisecond):
	}
	close(writer.release)
	select {
	case <-served:
	case <-time.After(time.Second):
		t.Fatal("stream did not join after writing")
	}
	select {
	case <-advanced:
	default:
		t.Fatal("written chunk did not release production")
	}
}

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
	ctx, cancel := context.WithCancel(request.Context())
	defer cancel()
	request = request.WithContext(ctx)
	served := make(chan struct{})
	go func() {
		defer close(served)
		serveStream(recorder, request, ack, events, "runs.start", cancel)
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
