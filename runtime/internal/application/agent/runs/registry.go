package runs

import (
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

type Record struct {
	ID         identity.RunID
	SegmentID  identity.SegmentID
	ExecutorID string
}

type liveSegment struct {
	record Record
	owner  *runTreeOwner
}

type registry struct {
	opening sync.RWMutex
	mu      sync.Mutex
	runs    map[identity.RunID]liveSegment
}

func (r *registry) Open(record Record, owner *runTreeOwner, commit func() error) error {
	r.opening.Lock()
	defer r.opening.Unlock()
	if err := commit(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()
	r.runs[record.ID] = liveSegment{record: record, owner: owner}
	return nil
}

// An older pump must never delete a newer Segment entry merely because both share
// the Run ID.
func (r *registry) RemoveSegment(id identity.RunID, segmentID identity.SegmentID) (segment liveSegment, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok = r.runs[id]
	if !ok || segment.record.SegmentID != segmentID {
		return liveSegment{}, false
	}
	delete(r.runs, id)
	return segment, ok
}

func (r *registry) Get(id identity.RunID) (liveSegment, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok := r.runs[id]
	return segment, ok
}

func (r *registry) Running(id identity.RunID) (liveSegment, bool) {
	r.opening.RLock()
	defer r.opening.RUnlock()
	return r.Get(id)
}

func (r *registry) initLocked() {
	if r.runs == nil {
		r.runs = map[identity.RunID]liveSegment{}
	}
}
