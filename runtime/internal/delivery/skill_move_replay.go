package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/skills"
)

func skillMoveReplay(to skills.Lifecycle) func(*Handler, context.Context, protocol.SkillNameRequest, keyedCall) protocol.OperationResult {
	return func(s *Handler, ctx context.Context, input protocol.SkillNameRequest, call keyedCall) protocol.OperationResult {
		payload, err := call.replay.InvokeRecoverable(ctx, call.key, call.fingerprint, call.admit, func(retained bool) error {
			if retained {
				return s.workspaceSkills.RecoverMove(ctx)
			}
			receipt := operation.ReplayClaim{Key: call.key, Fingerprint: call.fingerprint}
			return s.workspaceSkills.Move(ctx, input.Name, to, receipt)
		}, EncodeSkillMoveReceipt)
		return call.result(ctx, payload, err)
	}
}

func EncodeSkillMoveReceipt(cause error) ([]byte, error) {
	return encodeAcknowledgement(mapSkillError(cause))
}
