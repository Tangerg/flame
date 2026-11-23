package http

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

func (s *Server) instrumentRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		w.Header().Set(headerRequestID, newRequestID())
		w.Header().Set(headerServer, s.serverID)

		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				telemetry.HTTPRequestMethodKey.String(r.Method),
			),
		)
		r = r.WithContext(ctx)

		response := &recordingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			route := httpRoute(r)
			span.SetName(r.Method + " " + route)
			span.SetAttributes(
				telemetry.HTTPRouteKey.String(route),
				telemetry.HTTPResponseStatusCodeKey.Int(response.status),
				attribute.Int64("duration_ms", time.Since(start).Milliseconds()),
				telemetry.HTTPResponseBodySizeKey.Int(response.bytes),
			)
			if response.status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(response.status))
			}
			span.End()
		}()

		defer func() {
			if recovered := recover(); recovered != nil {
				err := handlerPanicError(recovered)
				slog.ErrorContext(ctx, "http: handler panicked",
					telemetry.HTTPRequestMethod(r.Method), telemetry.HTTPRoute(httpRoute(r)),
					telemetry.Panic(recovered))
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
				if !response.wroteHeader {
					writeProblem(response, problemInternalError, "the transport failed to process the request")
				}
			}
		}()

		next.ServeHTTP(response, r)
	})
}

func httpRoute(request *http.Request) string {
	if rc := chi.RouteContext(request.Context()); rc != nil {
		if pattern := rc.RoutePattern(); pattern != "" {
			return pattern
		}
	}
	return "unmatched"
}

const requestIDPrefix = "req_"

func newRequestID() string { return requestIDPrefix + rand.Text() }

type recordingResponseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (r *recordingResponseWriter) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.ResponseWriter.WriteHeader(status)
	// Informational responses leave the final response available; an upgrade
	// consumes it because the connection no longer carries ordinary HTTP.
	if status >= http.StatusContinue && status < http.StatusOK && status != http.StatusSwitchingProtocols {
		return
	}
	r.wroteHeader = true
	r.status = status
}

func (r *recordingResponseWriter) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func (r *recordingResponseWriter) Flush() { _ = r.FlushError() }

// ResponseController must receive an underlying flush failure instead of
// mistaking this wrapper's Flusher method for a successful transport write.
func (r *recordingResponseWriter) FlushError() error {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(r.ResponseWriter).Flush()
}

func (r *recordingResponseWriter) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func handlerPanicError(recovered any) error {
	if cause, ok := recovered.(error); ok {
		return fmt.Errorf("http handler panicked: %w", cause)
	}
	return fmt.Errorf("http handler panicked: %v", recovered)
}
