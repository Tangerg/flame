package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"reflect"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/go-sdk/jsonrpc"
	"github.com/Tangerg/sse"
	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

// TransportError is an HTTP-envelope failure. It says nothing about an
// operation's outcome and is deliberately separate from protocol.ProblemError.
type TransportError struct {
	StatusCode int
	Type       string
	RequestID  string
}

func (e *TransportError) Error() string {
	return fmt.Sprintf("runtime: transport returned http %d", e.StatusCode)
}

func (r *Client) invokeOperation(ctx context.Context, name string, parameters any, options callConfig) (result callResult, err error) {
	meta, found := lookupMethod(name)
	if !found {
		return callResult{}, protocol.NewFailure(protocol.ErrMethodNotFound, "unknown runtime operation")
	}
	operation, err := invocationOperation(meta, parameters)
	if err != nil {
		return callResult{}, err
	}
	call, err := r.begin(ctx)
	if err != nil {
		return callResult{}, err
	}
	streaming := false
	dispatched := false
	defer func() {
		if !streaming {
			call.finish(nil)
		}
		if dispatched && operation == "command" && err != nil && !knownTransportRefusal(err) {
			err = errors.Join(ErrAcknowledgementUnknown, err)
		}
	}()
	id, err := jsonrpc.MakeID(rand.Text())
	if err != nil {
		return callResult{}, err
	}
	body, err := encodeRemoteRequest(id, name, parameters, options.RequestMeta)
	if err != nil {
		return callResult{}, protocol.InvalidParameters(err)
	}
	request, err := http.NewRequestWithContext(call.ctx, http.MethodPost, r.endpoint, bytes.NewReader(body))
	if err != nil {
		return callResult{}, err
	}
	// A response lost after dispatch is an unknown acknowledgement. Recovery
	// belongs to the caller's retained command identity, never net/http replay.
	request.GetBody = nil
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if r.callbackCall != "" {
		request.Header.Set("Authorization", r.token)
		request.Header.Set(qimumcp.CallHeader, r.callbackCall)
		if invocation, invocationErr := qimumcp.InvocationFromContext(call.ctx); invocationErr == nil {
			value, encodeErr := json.Marshal(invocation)
			if encodeErr != nil {
				return callResult{}, encodeErr
			}
			request.Header.Set(qimumcp.InvocationHeader, string(value))
		}
	} else if r.token != "" {
		request.Header.Set("Authorization", "Bearer "+r.token)
	}
	for header, value := range map[string]string{
		"Idempotency-Key":       options.IdempotencyKey,
		"Idempotency-Namespace": options.IdempotencyNamespace,
		"Last-Event-Id":         options.AfterEventID,
	} {
		if value != "" {
			request.Header.Set(header, value)
		}
	}
	if err := call.ctx.Err(); err != nil {
		return callResult{}, context.Cause(call.ctx)
	}
	dispatched = true
	response, err := r.http.Do(request)
	if err != nil {
		return callResult{}, call.readError(err)
	}
	if err := call.attach(response.Body); err != nil {
		return callResult{}, err
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK {
		if r.callbackCall != "" && response.StatusCode == http.StatusGone {
			return callResult{}, qimumcp.ErrStaleCall
		}
		return callResult{}, call.readError(decodeTransportError(response, mediaType, r.maxMessageBytes))
	}
	if err != nil {
		return callResult{}, invalidRemote("response content type is invalid")
	}
	if mediaType == "application/json" {
		encoded, err := readRemoteMessage(response.Body, r.maxMessageBytes)
		if err != nil {
			return callResult{}, call.readError(err)
		}
		result, err := decodeRemoteResponse(encoded, id, meta)
		if err == nil && result.Failure == nil && meta.Kind == "stream" {
			err = invalidRemote("stream acknowledgement arrived without an event stream")
		}
		return result, err
	}
	if mediaType != "text/event-stream" || meta.Kind != "stream" {
		return callResult{}, invalidRemote("response content type does not match the operation")
	}
	// SSE's web-standard decoder repairs malformed UTF-8. Runtime JSON is exact,
	// so reject those bytes before the SSE reader can replace their meaning.
	reader := sse.NewReader(transform.NewReader(response.Body, encoding.UTF8Validator))
	reader.MaxLineBytes = math.MaxInt - 2
	if r.maxMessageBytes > 0 {
		reader.MaxLineBytes = r.maxMessageBytes
		reader.MaxEventBytes = r.maxMessageBytes
	}
	for frame, err := range reader.Messages() {
		if err != nil {
			return callResult{}, call.readError(err)
		}
		if frame.ID != "" || frame.Event != "message" {
			return callResult{}, invalidRemote("stream acknowledgement has invalid sse metadata")
		}
		result, err := decodeRemoteResponse(frame.Data, id, meta)
		if err != nil || result.Failure != nil {
			return result, err
		}
		if err := validateStreamIdentity(parameters, result.Value); err != nil {
			return callResult{}, err
		}
		result.Events = remoteEvents(call, reader, meta.Event, operation, result.Value)
		streaming = true
		return result, nil
	}
	return callResult{}, call.readError(io.ErrUnexpectedEOF)
}

func encodeRemoteRequest(id jsonrpc.ID, name string, parameters any, metadata protocol.RequestMeta) ([]byte, error) {
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return nil, err
	}
	var object map[string]jsontext.Value
	if err := json.Unmarshal(encoded, &object); err != nil {
		return nil, err
	}
	meta, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	object["_meta"] = meta
	encoded, err = json.Marshal(object, json.Deterministic(true))
	if err != nil {
		return nil, err
	}
	return jsonrpc.EncodeMessage(&jsonrpc.Request{ID: id, Method: name, Params: encoded})
}

func decodeRemoteResponse(encoded []byte, id jsonrpc.ID, method methodMeta) (callResult, error) {
	message, err := protocol.DecodeRPCMessage(encoded)
	if err != nil {
		return callResult{}, invalidRemote("invalid json-rpc response")
	}
	response, ok := message.(*jsonrpc.Response)
	if !ok || response.ID.Raw() != id.Raw() {
		return callResult{}, invalidRemote("response does not acknowledge this request")
	}
	if response.Error != nil {
		rpcError, ok := errors.AsType[*jsonrpc.Error](response.Error)
		if !ok {
			return callResult{}, invalidRemote("invalid operation error envelope")
		}
		var problem protocol.ProblemData
		if err := protocol.DecodeResponse(rpcError.Data, &problem); err != nil {
			return callResult{}, invalidRemote("invalid operation problem")
		}
		code, known := problemCodes[problem.Type]
		if !known || int64(code) != rpcError.Code || rpcError.Message != problem.Type {
			return callResult{}, invalidRemote("operation problem does not match its envelope")
		}
		failure, err := protocol.DecodeFailure(problem)
		if err != nil {
			return callResult{}, invalidRemote("invalid operation problem data")
		}
		return callResult{Failure: failure}, nil
	}
	value, err := method.decodeResult(response.Result)
	if err != nil {
		return callResult{}, invalidRemote("response violates the runtime contract")
	}
	return callResult{Value: value}, nil
}

func decodeRemoteValue(encoded []byte, valueType reflect.Type) (any, error) {
	value := reflect.New(valueType)
	if err := protocol.DecodeResponse(encoded, value.Interface()); err != nil {
		return nil, invalidRemote("invalid typed response")
	}
	decoded := value.Elem().Interface()
	if err := protocol.ValidateWireTree(decoded); err != nil {
		return nil, invalidRemote("response violates the runtime contract")
	}
	return decoded, nil
}

func readRemoteMessage(reader io.Reader, limit int) ([]byte, error) {
	if limit > 0 && limit < math.MaxInt {
		reader = io.LimitReader(reader, int64(limit)+1)
	}
	encoded, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(encoded) > limit {
		return nil, invalidRemote("response exceeds the message limit")
	}
	return encoded, nil
}

func decodeTransportError(response *http.Response, mediaType string, limit int) error {
	failure := &TransportError{StatusCode: response.StatusCode, RequestID: response.Header.Get("Request-Id")}
	if mediaType != "application/problem+json" {
		return failure
	}
	encoded, err := readRemoteMessage(response.Body, limit)
	if err != nil {
		return errors.Join(failure, err)
	}
	var problem transportProblem
	if err := protocol.DecodeResponse(encoded, &problem); err == nil && problem.Status == response.StatusCode {
		failure.Type = problem.Type
	}
	return failure
}

func knownTransportRefusal(err error) bool {
	if errors.Is(err, qimumcp.ErrStaleCall) {
		return true
	}
	problem, ok := errors.AsType[*TransportError](err)
	if !ok {
		return false
	}
	return refusalProblems[problem.Type] == problem.StatusCode
}

func invalidRemote(detail string) error {
	return fmt.Errorf("runtime: %s: %w", detail, protocol.ErrInvalidResponse)
}
