package maintenance

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

func NewLiveStateSnapshotter(shells *exec.Shells) LiveStateSnapshotter {
	if shells == nil {
		return nil
	}
	return func(_ context.Context, sessionID identity.SessionID) LiveStateSnapshot {
		var snap LiveStateSnapshot
		for _, sh := range shells.RetainedForSession(sessionID.String()) {
			snap.Shells = append(snap.Shells, RetainedShell{ID: sh.ID, Command: sh.Command})
		}
		return snap
	}
}
