package sessions

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

// DeletePlan removes exactly one addressed conversation. User-created forks
// are independent conversations and delegated work belongs to child Runs.
type DeletePlan struct {
	sessionID resourceid.SessionID
}

// NewDeletePlan owns one canonical Session identity before lifecycle effects.
func NewDeletePlan(sessionID string) (DeletePlan, error) {
	id, err := resourceid.ParseSession(sessionID)
	if err != nil {
		return DeletePlan{}, fmt.Errorf("sessions: delete plan: %w", err)
	}
	return DeletePlan{sessionID: id}, nil
}

// IsZero reports whether no delete plan was constructed.
func (d DeletePlan) IsZero() bool { return d.sessionID.String() == "" }

// SessionID returns the exact durable owner to remove.
func (d DeletePlan) SessionID() string { return d.sessionID.String() }
