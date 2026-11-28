package http

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"github.com/Tangerg/go-sdk/jsonrpc"
)

type rpcHandler struct {
	router *dispatch.Router
	store  string
}

func NewRPCHandler(endpoint *delivery.Endpoint) (http.Handler, error) {
	router, err := dispatch.New(endpoint)
	if err != nil {
		return nil, err
	}
	return &rpcHandler{router: router, store: endpoint.StoreNamespace()}, nil
}

const maxRPCBodyBytes = 4 << 20

func (s *rpcHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, name := range requestHeaders() {
		values := r.Header.Values(name)
		if len(values) > 1 {
			writeProblem(w, problemInvalidRequest, name+" header must occur at most once")
			return
		}
		if len(values) == 1 && values[0] == "" &&
			(name == headerIdempotencyKey || name == headerIdempotencyNamespace || name == headerRuntimeStore) {
			writeProblem(w, problemInvalidRequest, name+" header must not be empty")
			return
		}
	}
	if expected := r.Header.Get(headerRuntimeStore); expected != "" {
		if expected != s.store {
			writeProblem(w, problemRuntimeStoreMismatch, "the endpoint no longer identifies the registered Runtime store")
			return
		}
		w.Header().Set(headerRuntimeStore, s.store)
	}
	lifetime, cancel := context.WithCancel(r.Context())
	defer cancel()
	r = r.WithContext(lifetime)
	if ct := strings.TrimSpace(r.Header.Get(headerContentType)); ct != "" && !isJSONMediaType(ct) {
		writeProblem(w, problemUnsupportedMediaType, "content-type must be application/json")
		return
	}
	hint := r.Header.Get(headerMethod)
	body, err := readRPCBody(r, hint)
	if errors.Is(err, errRPCBodyTooLarge) {
		writeProblem(w, problemRequestTooLarge, err.Error())
		return
	}
	if err != nil {
		recordError(r.Context(), "rpc.read-request", err)
		writeProblem(w, problemInvalidRequest, "request body could not be read")
		return
	}

	message, err := protocol.DecodeRPCMessage(body)
	if err != nil {
		writeProblem(w, problemInvalidRequest, "invalid JSON-RPC message: "+err.Error())
		return
	}
	request, ok := message.(*jsonrpc.Request)
	if !ok {
		writeProblem(w, problemInvalidRequest, "POST /v2/rpc accepts only JSON-RPC requests and notifications")
		return
	}

	if hint != "" && hint != request.Method {
		writeProblem(w, problemInvalidRequest, "method header does not match the RPC envelope")
		return
	}
	methodLabel := request.Method
	if !request.IsCall() {
		w.Header().Set(headerMethod, methodLabel)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	options := protocol.CallMetadata{
		IdempotencyKey:       r.Header.Get(headerIdempotencyKey),
		IdempotencyNamespace: r.Header.Get(headerIdempotencyNamespace),
		AfterEventID:         r.Header.Get(headerLastEventID),
	}
	reply := s.router.Call(r.Context(), request, options)
	if reply.EventStream != nil {
		serveStream(w, r, &reply.Response, reply.EventStream, methodLabel, cancel)
		return
	}

	writeRPCResponse(w, r, &reply.Response, methodLabel)
}

func writeRPCResponse(w http.ResponseWriter, r *http.Request, response *jsonrpc.Response, methodLabel string) {
	responseBytes, err := jsonrpc.EncodeMessage(response)
	if err != nil {
		recordError(r.Context(), "rpc.encode-response", err,
			telemetry.RPCMethodKey.String(methodLabel),
		)
		writeProblem(w, problemResponseEncodingFailed, "the transport could not encode the RPC response")
		return
	}
	w.Header().Set(headerContentType, protocol.RPCDocumentMediaType+"; charset=utf-8")
	w.Header().Set(headerMethod, methodLabel)
	w.WriteHeader(protocol.RPCCallStatus)
	if _, err := w.Write(responseBytes); err != nil {
		recordError(r.Context(), "rpc.write-response", err,
			telemetry.RPCMethodKey.String(methodLabel),
		)
	}
}

var errRPCBodyTooLarge = errors.New("request body exceeds limit")

func readRPCBody(r *http.Request, methodHint string) ([]byte, error) {
	limit := int64(maxRPCBodyBytes)
	if methodHint == delivery.SessionsImport.String() {
		limit += protocol.MaximumSessionArtifactBytes
	}
	if r.ContentLength > limit {
		return nil, errRPCBodyTooLarge
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if _, tooLarge := errors.AsType[*http.MaxBytesError](err); tooLarge || int64(len(body)) > limit {
		return nil, errRPCBodyTooLarge
	}
	return body, err
}

const transportProblemNamespace = "urn:flame:transport:"

type problemKind struct {
	name    string
	status  int
	refusal bool
	noStore bool
}

var (
	problemUnsupportedMediaType   = problemKind{name: "unsupported_media_type", status: http.StatusUnsupportedMediaType, refusal: true}
	problemRequestTooLarge        = problemKind{name: "request_too_large", status: http.StatusRequestEntityTooLarge, refusal: true}
	problemInvalidRequest         = problemKind{name: protocol.ProblemInvalidRequest, status: http.StatusBadRequest, refusal: true}
	problemForbiddenOrigin        = problemKind{name: "forbidden_origin", status: http.StatusForbidden, refusal: true, noStore: true}
	problemRuntimeStoreMismatch   = problemKind{name: "runtime_store_mismatch", status: http.StatusConflict, refusal: true, noStore: true}
	problemUnauthorized           = problemKind{name: "unauthorized", status: http.StatusUnauthorized, refusal: true, noStore: true}
	problemResponseEncodingFailed = problemKind{name: "response_encoding_failed", status: http.StatusInternalServerError}
	problemInternalError          = problemKind{name: protocol.ProblemInternalError, status: http.StatusInternalServerError}

	problemKinds = []problemKind{
		problemUnsupportedMediaType,
		problemRequestTooLarge,
		problemInvalidRequest,
		problemUnauthorized,
		problemForbiddenOrigin,
		problemRuntimeStoreMismatch,
		problemResponseEncodingFailed,
		problemInternalError,
	}
)

func (k problemKind) problemType() string { return transportProblemNamespace + k.name }

func writeProblem(w http.ResponseWriter, kind problemKind, detail string) {
	w.Header().Set(headerContentType, protocol.HTTPProblemMediaType+"; charset=utf-8")
	if kind.noStore {
		w.Header().Set(headerCacheControl, cacheNoStore)
	}
	w.WriteHeader(kind.status)
	// The status line is already on the wire, so a failed body write has no
	// remaining way to change what this response says.
	_ = json.MarshalWrite(w, protocol.HTTPProblem{
		Type:   kind.problemType(),
		Title:  http.StatusText(kind.status),
		Status: kind.status,
		Detail: detail,
	})
}

func isJSONMediaType(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == protocol.RPCDocumentMediaType
}

func RefusalProblems() map[string]int {
	values := make(map[string]int)
	for _, kind := range problemKinds {
		if kind.refusal {
			values[kind.problemType()] = kind.status
		}
	}
	return values
}
