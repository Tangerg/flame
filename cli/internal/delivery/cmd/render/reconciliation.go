package render

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// resolveSnapshotRun selects the already accepted run from a cold projection.
// Falling back to the latest run is reserved for direct renderer use where no
// Begin call established an identity.
func resolveSnapshotRun(snapshot agent.SessionSnapshot, runID string) (runtimeprotocol.RunRef, error) {
	if runID == "" {
		latest, ok := snapshot.LatestRun()
		if !ok {
			return runtimeprotocol.RunRef{}, errors.New("snapshot has no run")
		}
		return latest, nil
	}
	if err := runtimeprotocol.ValidateRunID(runID); err != nil {
		return runtimeprotocol.RunRef{}, fmt.Errorf("snapshot run: %w", err)
	}
	run, ok := snapshot.RunByID(runID)
	if !ok {
		return runtimeprotocol.RunRef{}, fmt.Errorf("snapshot does not contain run %s", runID)
	}
	return run, nil
}
