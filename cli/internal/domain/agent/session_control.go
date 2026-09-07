package agent

import (
	"errors"
	"fmt"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// RollbackSession keeps ToRunID and every earlier root run. An empty ToRunID
// clears all history; file restoration therefore requires a concrete boundary.
type RollbackSession struct {
	CommandID CommandID
	SessionID string
	ToRunID   string
	Scope     runtimeprotocol.RestoreType
}

func (r RollbackSession) Validate() error {
	var problems []error
	if r.CommandID != "" {
		if err := r.CommandID.Validate(); err != nil {
			problems = append(problems, err)
		}
	}
	if err := (runtimeprotocol.RollbackSessionRequest{
		SessionID: r.SessionID, ToRunID: r.ToRunID, RestoreType: r.Scope,
	}).ValidateWire(); err != nil {
		problems = append(problems, err)
	}
	if err := errors.Join(problems...); err != nil {
		return fmt.Errorf("rollback session: %w", err)
	}
	return nil
}

func (r RollbackSession) RestoresFiles() bool {
	return r.Scope == runtimeprotocol.RestoreFiles || r.Scope == runtimeprotocol.RestoreBoth
}

func (r RollbackSession) FilesOnly() bool { return r.Scope == runtimeprotocol.RestoreFiles }

func (r RollbackSession) HistoryOnly() bool {
	return r.Scope == "" || r.Scope == runtimeprotocol.RestoreHistory
}

type RollbackResult struct {
	Session       Session
	DroppedRunIDs []string
}

func (r RollbackResult) Validate() error {
	if err := r.Session.Validate(); err != nil {
		return fmt.Errorf("rollback result: %w", err)
	}
	seen := make(map[string]struct{}, len(r.DroppedRunIDs))
	for index, runID := range r.DroppedRunIDs {
		if err := runtimeprotocol.ValidateRunID(runID); err != nil {
			return fmt.Errorf("rollback result dropped run %d: %w", index+1, err)
		}
		if _, duplicate := seen[runID]; duplicate {
			return fmt.Errorf("rollback result repeats run %q", runID)
		}
		seen[runID] = struct{}{}
	}
	return nil
}
