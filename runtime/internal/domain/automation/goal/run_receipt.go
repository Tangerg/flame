package goal

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

type RunReceipt struct {
	RunID         identity.RunID
	SessionID     identity.SessionID
	IncarnationID goalref.IncarnationID
	Revision      uint64
	Outcome       run.Outcome
	FinishedAt    time.Time
}

func (r RunReceipt) Owns(g Goal) bool {
	return r.SessionID == g.SessionID() && r.IncarnationID == g.IncarnationID() && r.Revision == g.Revision()
}
