package transport

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
)

// Message constructors use the SDK's public transport aliases so production
// callers do not have to reach into its internal JSON-RPC package.

// NewNotification builds a no-id Request — JSON-RPC Notification.
// Notifications get no response on the wire; senders are expected to
// fire-and-forget.
func NewNotification(method string, params any) (*Request, error) {
	encodedParams, err := marshalPayload(params)
	if err != nil {
		return nil, err
	}
	return &Request{Method: method, Params: encodedParams}, nil
}

// NewResponseResult builds a successful Response for the given id.
// The result is marshaled to JSON; an encoding failure surfaces as a
// CodeInternalError reply.
func NewResponseResult(id ID, result any) (*Response, error) {
	encodedResult, err := marshalPayload(result)
	if err != nil {
		return nil, err
	}
	return &Response{ID: id, Result: encodedResult}, nil
}

// NewResponseError builds an error Response for the given id.
func NewResponseError(id ID, rpcError *Error) *Response {
	return &Response{ID: id, Error: rpcError}
}

// NewError builds an RPC error with a caller-selected message and structured
// data — useful when a downstream error's detail is safe to surface.
func NewError(code int, message string, data jsontext.Value) *Error {
	return &Error{Code: int64(code), Message: message, Data: data}
}

// marshalPayload JSON-encodes a params/result value. Nil returns nil so
// the field omits on the wire.
func marshalPayload(value any) (jsontext.Value, error) {
	if value == nil {
		return nil, nil
	}
	if encoded, ok := value.(jsontext.Value); ok {
		return encoded, nil
	}
	// A response carrying a map — tool arguments, a tool result, a JSON Schema —
	// must reach the client as the same bytes every time, which is what the
	// previous encoder did implicitly and what v2 does only on request.
	return json.Marshal(value, json.Deterministic(true))
}
