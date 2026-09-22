package agentexec

import (
	"sync"
	"time"
)

// interactionChildProjection serializes projection of completed Delegate
// children with installation of a committed waiting-subtree replacement.
type interactionChildProjection struct{ mu sync.Mutex }

func (i *interactionChildProjection) lock()   { i.mu.Lock() }
func (i *interactionChildProjection) unlock() { i.mu.Unlock() }

// interactionSegmentClock owns the adapter-side start of the current product
// Segment. The Agent Process may survive several resumed Segments.
type interactionSegmentClock struct {
	mu        sync.Mutex
	startedAt time.Time
}

func (i *interactionSegmentClock) start() {
	i.mu.Lock()
	i.startedAt = time.Now().UTC()
	i.mu.Unlock()
}

func (i *interactionSegmentClock) duration(processStartedAt, finishedAt time.Time) time.Duration {
	i.mu.Lock()
	segmentStartedAt := i.startedAt
	i.mu.Unlock()
	return interactionSegmentDuration(processStartedAt, segmentStartedAt, finishedAt)
}
