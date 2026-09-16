package runs

import (
	"sync"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Record is the observable state of an active run segment.
type Record struct {
	ID             string
	SegmentID      string
	SessionID      string
	CreatedAt      time.Time
	ExecutorID     string
	ModelSelection modelref.Selection
	// Capabilities is the Run's frozen optional behavior, carried on the live
	// record so an insufficient subscriber is refused before attachment.
	Capabilities run.Capabilities
	CancelReason string
}

// liveSegment is the coordinator's process-local state for a currently active
// run. The registry only ever manages Run-tree owners, so making it generic would
// hide its actual lifecycle ownership. A live segment always has its owner:
// admission opens the entry from a startup that already built one.
type liveSegment struct {
	record Record
	owner  *runTreeOwner
}

// registry is the process-local registry of live run segments. Session
// admission is owned separately by application/ownership because Sessions and
// Runs share that invariant; durable run history lives in transcript.
//
// Its zero value is usable.
type registry struct {
	opening sync.RWMutex
	mu      sync.Mutex
	runs    map[string]liveSegment
}

// Open publishes the process-local owner together with its durable opening.
// Running-owner readers cannot pass a committed opening without its owner.
func (r *registry) Open(record Record, owner *runTreeOwner, commit func() error) error {
	r.opening.Lock()
	defer r.opening.Unlock()
	if err := commit(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initLocked()
	r.runs[record.ID] = liveSegment{record: cloneRecord(record), owner: owner}
	return nil
}

// RemoveSegment drops one exact completed segment and returns its former live
// state. A Run can resume onto a replacement Segment as soon as terminal
// maintenance releases admission; an older pump must never delete that newer
// registry entry merely because both Segments share the same Run ID.
func (r *registry) RemoveSegment(id, segmentID string) (segment liveSegment, ok bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok = r.runs[id]
	if !ok || segment.record.SegmentID != segmentID {
		return liveSegment{}, false
	}
	delete(r.runs, id)
	return segment, ok
}

// Get returns an active run segment.
func (r *registry) Get(id string) (liveSegment, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok := r.runs[id]
	segment.record = cloneRecord(segment.record)
	return segment, ok
}

// Running follows a durable Running read across the opening handoff. Waiting
// cancellation uses Get: it must remain free to reject a contending resume before
// that resume's commit returns.
func (r *registry) Running(id string) (liveSegment, bool) {
	r.opening.RLock()
	defer r.opening.RUnlock()
	return r.Get(id)
}

// MarkCancel records the human-facing cancel reason and returns the live run.
func (r *registry) MarkCancel(id, reason string) (liveSegment, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	segment, ok := r.runs[id]
	if !ok {
		return liveSegment{}, false
	}
	segment.record.CancelReason = reason
	r.runs[id] = segment
	segment.record = cloneRecord(segment.record)
	return segment, true
}

func cloneRecord(record Record) Record {
	record.Capabilities = record.Capabilities.Clone()
	return record
}

func (r *registry) initLocked() {
	if r.runs == nil {
		r.runs = map[string]liveSegment{}
	}
}
