package render

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/agent"
	cliidentity "github.com/Tangerg/flame/cli/internal/identity"
)

// resolveSnapshotRun selects the already accepted run from a cold projection.
// Falling back to the latest run is reserved for direct renderer use where no
// Begin call established an identity.
func resolveSnapshotRun(snapshot agent.SessionSnapshot, runID string) (agent.Run, error) {
	if runID == "" {
		latest, ok := snapshot.LatestRun()
		if !ok {
			return agent.Run{}, errors.New("snapshot has no run")
		}
		return latest, nil
	}
	if err := cliidentity.ValidateRun(runID); err != nil {
		return agent.Run{}, fmt.Errorf("snapshot run: %w", err)
	}
	run, ok := snapshot.RunByID(runID)
	if !ok {
		return agent.Run{}, fmt.Errorf("snapshot does not contain run %s", runID)
	}
	return run, nil
}
