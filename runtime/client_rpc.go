package runtime

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

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	"github.com/Tangerg/flame/runtime/protocol"
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

func (c *Client) invokeOperation(ctx context.Context, name delivery.Name, parameters any, options delivery.Options) (result delivery.Result, err error) {
	meta, found := delivery.Contract().Lookup(name)
	if !found {
		return delivery.Result{}, delivery.NewFailure(protocol.ErrMethodNotFound, "unknown runtime operation")
	}
	call, err := c.begin(ctx)
	if err != nil {
		return delivery.Result{}, err
	}
	streaming := false
	dispatched := false
	defer func() {
		if !streaming {
			call.finish(nil)
		}
		if dispatched && meta.Operation == delivery.OperationCommand && err != nil && !knownTransportRefusal(err) {
			err = errors.Join(ErrAcknowledgementUnknown, err)
		}
	}()
	id, err := jsonrpc.MakeID(rand.Text())
	if err != nil {
		return delivery.Result{}, err
	}
	body, err := encodeRemoteRequest(id, name, parameters, options.RequestMeta)
	if err != nil {
		return delivery.Result{}, delivery.InvalidParameters(err)
	}
	request, err := http.NewRequestWithContext(call.ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return delivery.Result{}, err
	}
	// A response lost after dispatch is an unknown acknowledgement. Recovery
	// belongs to the caller's retained command identity, never net/http replay.
	request.GetBody = nil
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
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
		return delivery.Result{}, context.Cause(call.ctx)
	}
	dispatched = true
	response, err := c.http.Do(request)
	if err != nil {
		return delivery.Result{}, call.readError(err)
	}
	if err := call.attach(response.Body); err != nil {
		return delivery.Result{}, err
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK {
		return delivery.Result{}, call.readError(decodeTransportError(response, mediaType, c.maxMessageBytes))
	}
	if err != nil {
		return delivery.Result{}, invalidRemote("response content type is invalid")
	}
	if mediaType == "application/json" {
		encoded, err := readRemoteMessage(response.Body, c.maxMessageBytes)
		if err != nil {
			return delivery.Result{}, call.readError(err)
		}
		result, err := decodeRemoteResponse(encoded, id, meta.Result)
		if err == nil && result.Failure == nil && meta.Kind == delivery.KindStream {
			err = invalidRemote("stream acknowledgement arrived without an event stream")
		}
		return result, err
	}
	if mediaType != "text/event-stream" || meta.Kind != delivery.KindStream {
		return delivery.Result{}, invalidRemote("response content type does not match the operation")
	}
	// SSE's web-standard decoder repairs malformed UTF-8. Runtime JSON is exact,
	// so reject those bytes before the SSE reader can replace their meaning.
	reader := sse.NewReader(transform.NewReader(response.Body, encoding.UTF8Validator))
	reader.MaxLineBytes = math.MaxInt - 2
	if c.maxMessageBytes > 0 {
		reader.MaxLineBytes = c.maxMessageBytes
		reader.MaxEventBytes = c.maxMessageBytes
	}
	for frame, err := range reader.Messages() {
		if err != nil {
			return delivery.Result{}, call.readError(err)
		}
		if frame.ID != "" || frame.Event != "message" {
			return delivery.Result{}, invalidRemote("stream acknowledgement has invalid sse metadata")
		}
		result, err := decodeRemoteResponse(frame.Data, id, meta.Result)
		if err != nil || result.Failure != nil {
			return result, err
		}
		if err := validateStreamIdentity(parameters, result.Value); err != nil {
			return delivery.Result{}, err
		}
		result.Events = remoteEvents(call, reader, meta.Event, result.Value)
		streaming = true
		return result, nil
	}
	return delivery.Result{}, call.readError(io.ErrUnexpectedEOF)
}

func encodeRemoteRequest(id transport.ID, name delivery.Name, parameters any, metadata protocol.RequestMeta) ([]byte, error) {
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
	return transport.EncodeMessage(&transport.Request{ID: id, Method: name.String(), Params: encoded})
}

func decodeRemoteResponse(encoded []byte, id transport.ID, resultType reflect.Type) (delivery.Result, error) {
	message, err := transport.DecodeMessage(encoded)
	if err != nil {
		return delivery.Result{}, invalidRemote("invalid json-rpc response")
	}
	response, ok := message.(*transport.Response)
	if !ok || response.ID.Raw() != id.Raw() {
		return delivery.Result{}, invalidRemote("response does not acknowledge this request")
	}
	if response.Error != nil {
		rpcError, ok := errors.AsType[*transport.Error](response.Error)
		if !ok {
			return delivery.Result{}, invalidRemote("invalid operation error envelope")
		}
		var problem protocol.ProblemData
		if err := transport.DecodeValue(rpcError.Data, &problem, "error.data"); err != nil {
			return delivery.Result{}, invalidRemote("invalid operation problem")
		}
		code, known := dispatch.ProblemCodes()[problem.Type]
		if !known || int64(code) != rpcError.Code || rpcError.Message != problem.Type {
			return delivery.Result{}, invalidRemote("operation problem does not match its envelope")
		}
		failure, err := delivery.DecodeFailure(problem)
		if err != nil {
			return delivery.Result{}, invalidRemote("invalid operation problem data")
		}
		return delivery.Result{Failure: failure}, nil
	}
	if resultType == nil {
		resultType = reflect.TypeFor[struct{}]()
	}
	value, err := decodeRemoteValue(response.Result, resultType)
	return delivery.Result{Value: value}, err
}

func decodeRemoteValue(encoded []byte, valueType reflect.Type) (any, error) {
	value := reflect.New(valueType)
	if err := transport.DecodeValue(encoded, value.Interface(), "result"); err != nil {
		return nil, invalidRemote("invalid typed response")
	}
	if err := transport.ValidateRequiredFields(encoded, valueType, "result"); err != nil {
		return nil, invalidRemote("response omits a required field")
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
	var problem struct {
		Type      string `json:"type"`
		Title     string `json:"title"`
		Status    int    `json:"status"`
		Detail    string `json:"detail"`
		RequestID string `json:"requestId,omitempty"`
	}
	if err := transport.DecodeValue(encoded, &problem, "problem"); err == nil && problem.Status == response.StatusCode {
		failure.Type = problem.Type
	}
	return failure
}

func knownTransportRefusal(err error) bool {
	problem, ok := errors.AsType[*TransportError](err)
	if !ok {
		return false
	}
	switch problem.Type {
	case "urn:flame:transport:invalid_request":
		return problem.StatusCode == http.StatusBadRequest
	case "urn:flame:transport:unauthorized":
		return problem.StatusCode == http.StatusUnauthorized
	case "urn:flame:transport:request_too_large":
		return problem.StatusCode == http.StatusRequestEntityTooLarge
	case "urn:flame:transport:unsupported_media_type":
		return problem.StatusCode == http.StatusUnsupportedMediaType
	default:
		return false
	}
}

func invalidRemote(detail string) error {
	return fmt.Errorf("runtime: %s: %w", detail, ErrInvalidResponse)
}
