package runs

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type Event struct {
	RunID     identity.RunID
	SegmentID identity.SegmentID
	Sequence  uint64
	Cursor    string
	Timestamp time.Time
	Payload   ProjectionEvent
}

func (e Event) Replayable() bool { return e.Payload != nil && e.Payload.Replayable() }

func (e Event) Terminal() bool { return e.Payload != nil && e.Payload.Terminal() }
