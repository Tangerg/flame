package agent

import (
	"fmt"

	runtimeprotocol "github.com/Tangerg/flame/runtime/protocol"
)

// RunQuery selects one cursor page in runtime order, newest first. An empty
// status set means all lifecycle states; descendants remain opt-in because
// their presence changes both topology and pagination.
type RunQuery struct {
	SessionID          string
	Statuses           []runtimeprotocol.RunStatus
	IncludeDescendants bool
	Cursor             string
	PageSize           PageSize
}

func (r RunQuery) Validate() error {
	if r.SessionID != "" {
		if err := runtimeprotocol.ValidateSessionID(r.SessionID); err != nil {
			return fmt.Errorf("run query: %w", err)
		}
	}
	statuses := r.Statuses
	if len(statuses) == 0 {
		statuses = nil
	}
	if err := runtimeprotocol.ValidateWireTree(runtimeprotocol.ListRunsRequest{Statuses: statuses}); err != nil {
		return fmt.Errorf("run query statuses %q: %w", r.Statuses, err)
	}
	if _, err := r.PageSize.Rows(); err != nil {
		return fmt.Errorf("run query: %w", err)
	}
	return nil
}
