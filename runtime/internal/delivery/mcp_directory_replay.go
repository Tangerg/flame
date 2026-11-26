package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
)

func directoryReplay[Params any](invoke func(*Handler, context.Context, Params, operation.ReplayClaim) error) func(*Handler, context.Context, Params, keyedCall) protocol.OperationResult {
	return func(s *Handler, ctx context.Context, in Params, call keyedCall) protocol.OperationResult {
		receipt := operation.ReplayClaim{Key: call.key, Fingerprint: call.fingerprint}
		payload, err := call.replay.InvokeRecoverable(ctx, call.key, call.fingerprint, func(retained bool) error {
			if retained {
				return s.mcp.RecoverDirectoryChange(ctx, receipt)
			}
			return invoke(s, ctx, in, receipt)
		}, EncodeMCPDirectoryReceipt)
		return call.result(ctx, payload, err)
	}
}

func EncodeMCPDirectoryReceipt(cause error) ([]byte, error) {
	if cause != nil {
		return encodeStoredOutcome(failed(projectError(wireMCPError(cause))))
	}
	return encodeStoredOutcome(protocol.OperationResult{Value: struct{}{}})
}
