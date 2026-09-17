package agentexec

import (
	"sync"
	"time"

	agent "github.com/Tangerg/scope/agent"
	corechat "github.com/Tangerg/scope/core/chat"
)

// interactionChildProjection serializes projection of completed Delegate
// children with installation of a committed waiting-subtree replacement.
type interactionChildProjection struct{ mu sync.Mutex }

func (i *interactionChildProjection) lock()   { i.mu.Lock() }
func (i *interactionChildProjection) unlock() { i.mu.Unlock() }

// interactionCommittedReplies owns assistant values already accepted by the
// authoritative Run projection until the corresponding Delegate closes.
type interactionCommittedReplies struct {
	mu      sync.Mutex
	byChild map[agent.ProcessID]corechat.Message
}

func newInteractionCommittedReplies() interactionCommittedReplies {
	return interactionCommittedReplies{byChild: make(map[agent.ProcessID]corechat.Message)}
}

func (i *interactionCommittedReplies) record(processID agent.ProcessID, message corechat.Message) {
	i.mu.Lock()
	i.byChild[processID] = message.Clone()
	i.mu.Unlock()
}

func (i *interactionCommittedReplies) lookup(processID agent.ProcessID) (corechat.Message, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	message, found := i.byChild[processID]
	return message.Clone(), found
}

func (i *interactionCommittedReplies) forget(processID agent.ProcessID) {
	i.mu.Lock()
	delete(i.byChild, processID)
	i.mu.Unlock()
}

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
