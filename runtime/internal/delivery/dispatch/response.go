package dispatch

import (
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/go-sdk/jsonrpc"
)

func responseResult(id jsonrpc.ID, result any) Result {
	response, err := protocol.NewRPCResult(id, result)
	if err != nil {
		// Response encoding is an infrastructure fault. Its serializer detail can
		// contain arbitrary implementation data, so the protocol exposes only a
		// stable client-safe problem.
		return responseError(id, problemError(protocol.ErrInternalError, "the runtime could not encode the response"))
	}
	return Result{Response: response}
}

func responseError(id jsonrpc.ID, rpcError *jsonrpc.Error) Result {
	return Result{Response: protocol.NewRPCErrorResponse(id, rpcError)}
}
