package http

import (
	"context"
	"fmt"
	"iter"
	"log/slog"
	"net/http"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"github.com/Tangerg/go-sdk/jsonrpc"
	"github.com/Tangerg/sse"
	"go.opentelemetry.io/otel/trace"
)

const heartbeatInterval = 15 * time.Second

// ctx cancellation cannot interrupt an in-flight net/http Write, so a stalled
// client would park this goroutine and block shutdown; a per-frame deadline makes
// one blocked write fail instead.
const streamWriteTimeout = 30 * time.Second

func serveStream(w http.ResponseWriter, r *http.Request, resp *jsonrpc.Response, events iter.Seq[dispatch.StreamFrame], methodLabel string, cancel context.CancelFunc) {
	w.Header().Set(headerCacheControl, cacheEventStream)
	w.Header().Set("X-Accel-Buffering", "no")
	if methodLabel != "" {
		w.Header().Set(headerMethod, methodLabel)
	}

	sw := sse.NewHTTPWriter(w)
	ctx := r.Context()
	rc := http.NewResponseController(w)
	setWriteDeadline := func() { _ = rc.SetWriteDeadline(time.Now().Add(streamWriteTimeout)) }

	setWriteDeadline()
	if err := writeSSEMessage(sw, "", resp); err != nil {
		return
	}

	type deliveryFrame struct {
		frame   dispatch.StreamFrame
		written chan struct{}
	}
	frames := make(chan deliveryFrame)
	settled := make(chan struct{})
	go func() {
		defer close(settled)
		defer close(frames)
		defer recoverStreamSourceDefect(ctx)
		for frame := range events {
			delivery := deliveryFrame{frame: frame, written: make(chan struct{})}
			select {
			case frames <- delivery:
			case <-ctx.Done():
				return
			}
			select {
			case <-delivery.written:
			case <-ctx.Done():
				return
			}
		}
	}()
	defer func() {
		cancel()
		<-settled
	}()

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case delivery, ok := <-frames:
			if !ok {
				return
			}
			setWriteDeadline()
			if err := writeSSEMessage(sw, delivery.frame.SSEID, delivery.frame.Notification); err != nil {
				return
			}
			close(delivery.written)
		case <-ticker.C:
			setWriteDeadline()
			if err := sw.Comment("heartbeat"); err != nil {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// The handler's recovery runs on the request goroutine and never sees a panic in
// the bridge goroutine that drives the source.
func recoverStreamSourceDefect(ctx context.Context) {
	recovered := recover()
	if recovered == nil {
		return
	}
	defect := fmt.Errorf("http: stream source panicked: %v", recovered)
	slog.ErrorContext(ctx, "http: stream source panicked",
		telemetry.Panic(recovered))
	trace.SpanFromContext(ctx).RecordError(defect)
}

func writeSSEMessage(sw *sse.Writer, eventID string, message jsonrpc.Message) error {
	body, err := jsonrpc.EncodeMessage(message)
	if err != nil {
		return err
	}
	return sw.Write(sse.Message{ID: eventID, Data: body})
}
