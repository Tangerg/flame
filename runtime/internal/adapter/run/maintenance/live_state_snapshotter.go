package maintenance

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/infra/process/exec"
)

// NewLiveStateSnapshotter adapts process-owned retained shells to the compactor's
// reminder source. Durable Session state has its own per-model-call projection
// and deliberately does not pass through summary maintenance.
func NewLiveStateSnapshotter(shells *exec.Shells) LiveStateSnapshotter {
	if shells == nil {
		return nil
	}
	return func(_ context.Context, sessionID string) LiveStateSnapshot {
		var snap LiveStateSnapshot
		for _, sh := range shells.RetainedForSession(sessionID) {
			snap.Shells = append(snap.Shells, RetainedShell{ID: sh.ID, Command: sh.Command})
		}
		return snap
	}
}
