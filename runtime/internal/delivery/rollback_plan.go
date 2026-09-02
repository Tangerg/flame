package delivery

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/protocol"
)

func rollbackScopeFromWire(in protocol.RollbackSessionRequest) (sessions.RestoreScope, error) {
	restoreType := in.RestoreType
	if restoreType == "" {
		restoreType = protocol.RestoreHistory
	}
	scope := sessions.RestoreScope(restoreType)
	if !scope.Valid() {
		return "", fmt.Errorf("%w: unknown restoreType %q", protocol.ErrInvalidParams, restoreType)
	}
	if scope.RestoresFiles() && in.ToRunID == "" {
		return "", fmt.Errorf("%w: restoreType %q requires toRunId", protocol.ErrInvalidParams, restoreType)
	}
	return scope, nil
}
