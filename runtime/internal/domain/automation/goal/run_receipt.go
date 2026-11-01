package goal

import (
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// A receipt addresses one committed terminal Run at the Goal version present
// when it ended. The terminal Run remains the owner of its outcome and usage.
type RunReceipt struct {
	RunID         string
	SessionID     string
	IncarnationID string
	Revision      uint64
	Outcome       run.Outcome
	FinishedAt    time.Time
}

func (r RunReceipt) Owns(g Goal) bool {
	return r.SessionID == g.SessionID() && r.IncarnationID == g.IncarnationID() && r.Revision == g.Revision()
}
