package sessions

import "fmt"

// DeletePlan removes exactly one addressed conversation. User-created forks
// are independent conversations and delegated work belongs to child Runs.
type DeletePlan struct {
	retirement ResourceRetirement
}

// NewDeletePlan owns one canonical Session identity before lifecycle effects.
func NewDeletePlan(sessionID string) (DeletePlan, error) {
	retirement, err := newResourceRetirement(sessionID)
	if err != nil {
		return DeletePlan{}, fmt.Errorf("sessions: delete plan: %w", err)
	}
	return DeletePlan{retirement: retirement}, nil
}

// SessionID returns the exact durable owner to remove.
func (d DeletePlan) SessionID() string { return d.retirement.SessionID() }

func (d DeletePlan) Retirement() ResourceRetirement { return d.retirement }
