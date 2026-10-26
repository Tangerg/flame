package protocol

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"

	jsonrpc "github.com/Tangerg/go-sdk/jsonrpc"
)

func NewRPCNotification(method string, params any) (*jsonrpc.Request, error) {
	if params == nil {
		return &jsonrpc.Request{Method: method}, nil
	}
	encodedParams, err := marshalPayload(params)
	if err != nil {
		return nil, err
	}
	return &jsonrpc.Request{Method: method, Params: encodedParams}, nil
}

func NewRPCResult(id jsonrpc.ID, result any) (*jsonrpc.Response, error) {
	encodedResult, err := marshalPayload(result)
	if err != nil {
		return nil, err
	}
	return &jsonrpc.Response{ID: id, Result: encodedResult}, nil
}

func NewRPCErrorResponse(id jsonrpc.ID, rpcError *jsonrpc.Error) *jsonrpc.Response {
	return &jsonrpc.Response{ID: id, Error: rpcError}
}

func NewRPCError(code int, message string, data jsontext.Value) *jsonrpc.Error {
	return &jsonrpc.Error{Code: int64(code), Message: message, Data: data}
}

func marshalPayload(value any) (jsontext.Value, error) {
	// A response carrying a map — tool arguments, a tool result, a JSON Schema —
	// must reach the client as the same bytes every time, which is what the
	// previous encoder did implicitly and what v2 does only on request.
	return json.Marshal(value, json.Deterministic(true))
}
