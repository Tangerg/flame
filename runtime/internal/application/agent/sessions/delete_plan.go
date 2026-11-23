package sessions

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type DeletePlan struct {
	retirement ResourceRetirement
}

func NewDeletePlan(sessionID identity.SessionID) (DeletePlan, error) {
	retirement, err := newResourceRetirement(sessionID)
	if err != nil {
		return DeletePlan{}, fmt.Errorf("sessions: delete plan: %w", err)
	}
	return DeletePlan{retirement: retirement}, nil
}

func (d DeletePlan) SessionID() identity.SessionID { return d.retirement.SessionID() }

func (d DeletePlan) Retirement() ResourceRetirement { return d.retirement }
