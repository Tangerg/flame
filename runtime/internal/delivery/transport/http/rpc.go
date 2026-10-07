package http

import (
	json "encoding/json/v2"
	"io"
	"mime"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
)

// maxRPCBodyBytes caps the JSON-RPC request body to avoid trivial
// DoS via a giant payload. 4 MB matches typical reasonable runs.start
// histories without trimming.
const maxRPCBodyBytes = 4 << 20

// serveRPC reads, dispatches, and serializes one JSON-RPC message.
// Wire encode/decode goes through transport.DecodeMessage /
// EncodeMessage — those wrap the MCP SDK's jsonrpc package, which
// owns the conformant JSON-RPC 2.0 implementation.
func (s *Server) serveRPC(w http.ResponseWriter, r *http.Request) {
	// Transport-layer preconditions: unsupported media
	// type ⇒ 415, oversized body ⇒ 413 — both rejected before we spend
	// effort decoding. Content-Type is only enforced when present (a
	// minimal client may omit it); when set it must be application/json.
	if ct := strings.TrimSpace(r.Header.Get(headerContentType)); ct != "" && !isJSONMediaType(ct) {
		writeProblem(w, problemUnsupportedMediaType, "content-type must be application/json", false)
		return
	}
	if r.ContentLength > maxRPCBodyBytes {
		writeProblem(w, problemRequestTooLarge, "request body exceeds limit", false)
		return
	}

	// Read one byte past the cap so a chunked / uncounted body that
	// overflows surfaces as 413 rather than silently truncating.
	body, err := io.ReadAll(io.LimitReader(r.Body, maxRPCBodyBytes+1))
	if err != nil {
		recordError(r.Context(), "rpc.read-request", err)
		writeProblem(w, problemInvalidRequest, "request body could not be read", false)
		return
	}
	if len(body) > maxRPCBodyBytes {
		writeProblem(w, problemRequestTooLarge, "request body exceeds limit", false)
		return
	}

	message, err := transport.DecodeMessage(body)
	if err != nil {
		writeProblem(w, problemInvalidRequest, "invalid JSON-RPC message: "+err.Error(), false)
		return
	}
	request, ok := message.(*transport.Request)
	if !ok {
		writeProblem(w, problemInvalidRequest, "POST /v2/rpc accepts only JSON-RPC requests and notifications", false)
		return
	}

	// Carry the streaming reconnect cursor (Last-Event-Id) out-of-band on
	// the ctx so runs.subscribe replays a run's retained replay window from that
	// point rather than re-sending it whole. Harmless for
	// non-streaming methods (they don't read it).
	ctx := transport.WithLastEventID(r.Context(), r.Header.Get(headerLastEventID))
	ctx = transport.WithIdempotencyKey(ctx, r.Header.Get(headerIdempotencyKey))
	ctx = transport.WithIdempotencyNamespace(ctx, r.Header.Get(headerIdempotencyNamespace))
	result := s.router.Dispatch(ctx, message)

	// Surface the request method for the X-Method header.
	methodLabel := request.Method

	// Client notifications are dispatched synchronously and acknowledged
	// with 204 No Content because processing has already completed.
	if result.Response == nil {
		if methodLabel != "" {
			w.Header().Set("X-Method", methodLabel)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Streaming method (stream opened) → the POST response body IS the
	// event stream. A pre-stream failure
	// (session_not_found / invalid_params …) leaves EventStream nil and
	// falls through to the single-shot application/json reply below.
	if result.EventStream != nil {
		s.serveStream(w, r, result.Response, result.EventStream, methodLabel)
		return
	}

	// A decoded JSON-RPC call always returns 200. Its result or error belongs to
	// the envelope; HTTP status is reserved for failures below this boundary.
	responseBytes, err := transport.EncodeMessage(result.Response)
	if err != nil {
		recordError(r.Context(), "rpc.encode-response", err,
			attribute.String("rpc.method", methodLabel),
		)
		writeProblem(w, problemResponseEncodingFailed, "the transport could not encode the RPC response", false)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if methodLabel != "" {
		w.Header().Set("X-Method", methodLabel)
	}
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(responseBytes); err != nil {
		recordError(r.Context(), "rpc.write-response", err,
			attribute.String("rpc.method", methodLabel),
		)
	}
}

// Problem is the application/problem+json body of a failure the transport
// answers itself, below any operation.
type Problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail"`
	RequestID string `json:"requestId,omitempty"`
}

// transportProblemNamespace separates an envelope failure from an operation's.
// A client matches the prefix to know the request never reached an operation,
// or that its response never left, so the two vocabularies stay independent
// even where they share a word.
const transportProblemNamespace = "urn:flame:transport:"

// problemKind is one problem the transport answers itself and the only status
// it is sent with.
type problemKind struct {
	name   string
	status int
	// refusal marks a problem answered before its request reached an
	// operation, so a command it answers was certainly not accepted.
	refusal bool
}

// The closed set of problems the transport answers with itself. invalid_request
// and internal_error are protocol vocabulary rather than transport vocabulary —
// a caller meets the same two symbols in an RPC error and a run outcome — so
// they are taken from their owner instead of spelled again here.
var (
	problemUnsupportedMediaType   = problemKind{name: "unsupported_media_type", status: http.StatusUnsupportedMediaType, refusal: true}
	problemRequestTooLarge        = problemKind{name: "request_too_large", status: http.StatusRequestEntityTooLarge, refusal: true}
	problemInvalidRequest         = problemKind{name: protocol.ProblemInvalidRequest, status: http.StatusBadRequest, refusal: true}
	problemUnauthorized           = problemKind{name: "unauthorized", status: http.StatusUnauthorized, refusal: true}
	problemResponseEncodingFailed = problemKind{name: "response_encoding_failed", status: http.StatusInternalServerError}
	problemInternalError          = problemKind{name: protocol.ProblemInternalError, status: http.StatusInternalServerError}

	problemKinds = []problemKind{
		problemUnsupportedMediaType,
		problemRequestTooLarge,
		problemInvalidRequest,
		problemUnauthorized,
		problemResponseEncodingFailed,
		problemInternalError,
	}
)

func (k problemKind) problemType() string { return transportProblemNamespace + k.name }

// RefusedBeforeDispatch reports whether a transport problem of problemType sent
// with status is one this transport answers before the request reaches an
// operation. Any other pairing, including a known type at a foreign status,
// proves nothing about whether an operation ran.
func RefusedBeforeDispatch(status int, problemType string) bool {
	for _, kind := range problemKinds {
		if kind.problemType() == problemType {
			return kind.refusal && kind.status == status
		}
	}
	return false
}

func writeProblem(w http.ResponseWriter, kind problemKind, detail string, noCache bool) {
	w.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	if noCache {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(kind.status)
	// The status line is already on the wire, so a failed body write has no
	// remaining way to change what this response says.
	_ = json.MarshalWrite(w, Problem{
		Type:      kind.problemType(),
		Title:     http.StatusText(kind.status),
		Status:    kind.status,
		Detail:    detail,
		RequestID: w.Header().Get("Request-Id"),
	})
}

// isJSONMediaType reports whether a Content-Type header denotes JSON.
// It tolerates parameters (e.g. "application/json; charset=utf-8") by
// parsing off the media type before comparing.
func isJSONMediaType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == "application/json"
}
