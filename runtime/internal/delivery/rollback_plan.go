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
		return "", NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown restoreType %q", restoreType))
	}
	if scope.RestoresFiles() && in.ToRunID == "" {
		return "", NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("restoreType %q requires toRunId", restoreType))
	}
	return scope, nil
}
