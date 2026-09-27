package render

import (
	"errors"
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// resolveSnapshotRun selects the already accepted run from a cold projection.
// Falling back to the latest run is reserved for direct renderer use where no
// Begin call established an identity.
func resolveSnapshotRun(snapshot conversation.SessionSnapshot, runID string) (conversation.Run, error) {
	if runID == "" {
		latest, ok := snapshot.LatestRun()
		if !ok {
			return conversation.Run{}, errors.New("snapshot has no run")
		}
		return latest, nil
	}
	if err := runtimeprotocol.ValidateRunID(runID); err != nil {
		return conversation.Run{}, fmt.Errorf("snapshot run: %w", err)
	}
	run, ok := snapshot.RunByID(runID)
	if !ok {
		return conversation.Run{}, fmt.Errorf("snapshot does not contain run %s", runID)
	}
	return run, nil
}
