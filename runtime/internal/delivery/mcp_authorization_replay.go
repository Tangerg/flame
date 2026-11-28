package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

func (s *Handler) replayMCPAuthorization(ctx context.Context, input protocol.CreateMCPAuthorizationAttemptRequest, call keyedCall) protocol.OperationResult {
	receipt := operation.ReplayClaim{Key: call.key, Fingerprint: call.fingerprint}
	payload, err := call.replay.InvokeRecoverable(ctx, call.key, call.fingerprint, call.admit, func(retained bool) error {
		if retained {
			return s.mcp.RecoverAuthorization(ctx, receipt)
		}
		server, err := mcpServerIDFromWire(input.Server)
		if err != nil {
			return err
		}
		return s.mcp.StartAuthorization(ctx, server, receipt)
	}, func(cause error) ([]byte, error) {
		return encodeAcknowledgement(wireMCPError(cause))
	})
	return call.result(ctx, payload, err)
}

func EncodeMCPAuthorizationReceipt(id mcpapp.AuthorizationAttemptID, cause error) ([]byte, error) {
	if cause != nil {
		return encodeAcknowledgement(wireMCPError(cause))
	}
	return encodeStoredOutcome(protocol.OperationResult{Value: &protocol.MCPAuthorizationAttemptRef{AttemptID: id.String()}})
}
