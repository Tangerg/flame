package delivery

import (
	"errors"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

var errSubscriptionAdmissionsClosed = errors.New("delivery: runtime subscriptions are closed")

const runtimeSubscriptionQueueCapacity = 64

// The hub never drops an invalidation or skips a sequence number: a slow subscriber's
// undelivered signals fold into one pending resync naming exactly the topics
// involved.
type runtimeEventHub struct {
	mu            sync.Mutex
	subscriptions map[*runtimeSubscription]struct{}
	closed        bool
}

func newRuntimeEventHub() *runtimeEventHub {
	return &runtimeEventHub{subscriptions: make(map[*runtimeSubscription]struct{})}
}

type runtimeSubscription struct {
	events    chan protocol.RuntimeEvent
	exhausted chan struct{}
	topics    map[protocol.RuntimeTopic]bool
	watchIDs  []string
	// Assigned when a frame is queued, never when produced or filtered, so a gap can
	// only mean the transport lost a frame.
	sequence          runtimeEventSequence
	stalledTopics     map[protocol.RuntimeTopic]bool
	stalledWatchIDs   []string
	stalledAllWatches bool
}

type runtimeEventSequence uint64

func (s runtimeEventSequence) next() (runtimeEventSequence, bool) {
	if uint64(s) >= protocol.MaximumRuntimeEventSequence {
		return 0, false
	}
	return s + 1, true
}

func (s runtimeEventSequence) wire() uint64 { return uint64(s) }

// register does not close the channel; its owner does after stopping every other
// writer, so a late broadcast cannot send on a closed channel.
func (w *runtimeEventHub) register(
	events chan protocol.RuntimeEvent,
	topics map[protocol.RuntimeTopic]bool,
	watchIDs []string,
) (*runtimeSubscription, func(), bool) {
	subscription := &runtimeSubscription{
		events:    events,
		exhausted: make(chan struct{}),
		topics:    topics,
		watchIDs:  slices.Clone(watchIDs),
	}
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return nil, nil, false
	}
	w.subscriptions[subscription] = struct{}{}
	w.mu.Unlock()
	return subscription, func() {
		w.mu.Lock()
		delete(w.subscriptions, subscription)
		w.mu.Unlock()
	}, true
}

func (w *runtimeEventHub) closeAdmissions() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

func (w *runtimeEventHub) publish(event protocol.RuntimeEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for subscription := range w.subscriptions {
		w.sendLocked(subscription, event)
	}
}

// Without this a pending resync would wait for the next unrelated signal, which on
// a quiet stream never comes.
func (w *runtimeEventHub) drained(subscription *runtimeSubscription) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, registered := w.subscriptions[subscription]; registered {
		subscription.flushStalledLocked()
	}
}

// Shares the broadcast serialization point so each subscriber's sequence stays
// ordered even when its git watcher races a global workspace event.
func (w *runtimeEventHub) publishTo(subscription *runtimeSubscription, event protocol.RuntimeEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, registered := w.subscriptions[subscription]; registered {
		w.sendLocked(subscription, event)
	}
}

func (*runtimeEventHub) sendLocked(subscription *runtimeSubscription, event protocol.RuntimeEvent) {
	if subscription.sequenceExhausted() {
		return
	}
	if event.Type == protocol.RuntimeResync {
		var deliver bool
		event, deliver = subscription.scopeResync(event)
		if !deliver {
			return
		}
	} else {
		topic := protocol.RuntimeTopic(event.Type)
		if !slices.Contains(protocol.RuntimeTopics(), topic) {
			// An invalid producer signal has no trustworthy narrowing scope. Preserve
			// client correctness by invalidating everything this subscription holds;
			// the encoder must never silently discard an internal shape violation.
			event = subscription.resyncEvent()
		} else if !subscription.topics[topic] {
			return
		} else if topic == protocol.TopicFilesChanged && event.WatchID != "" &&
			!slices.Contains(subscription.watchIDs, event.WatchID) {
			return
		}
	}
	// A pending resync goes first, and while it is pending this signal joins it: the
	// notice that frames were missed must not arrive after the frames that followed
	// them.
	if !subscription.flushStalledLocked() {
		if prepared, ok := subscription.prepareLocked(event); ok {
			subscription.stallLocked(prepared)
		}
		return
	}
	var prepared bool
	event, prepared = subscription.prepareLocked(event)
	if !prepared {
		return
	}
	if !subscription.offerPreparedLocked(event) {
		subscription.stallLocked(event)
	}
}

func (w *runtimeSubscription) scopeResync(
	event protocol.RuntimeEvent,
) (protocol.RuntimeEvent, bool) {
	if len(event.Topics) == 0 {
		return event, true
	}
	watchIDs, requested, wellFormed := resyncScope(event)
	if !wellFormed {
		return event, true
	}
	if len(event.WatchIDs) > 0 && !requested[protocol.TopicFilesChanged] {
		return w.resyncEvent(), true
	}
	if requested[protocol.TopicFilesChanged] && len(event.WatchIDs) > 0 {
		event.WatchIDs = w.heldWatchIDs(watchIDs)
		if len(event.WatchIDs) == 0 {
			delete(requested, protocol.TopicFilesChanged)
		}
	}
	scoped := make([]protocol.RuntimeTopic, 0, len(requested))
	for _, topic := range protocol.RuntimeTopics() {
		if requested[topic] && w.topics[topic] {
			scoped = append(scoped, topic)
		}
	}
	if len(scoped) == 0 {
		return protocol.RuntimeEvent{}, false
	}
	event.Topics = scoped
	if !slices.Contains(scoped, protocol.TopicFilesChanged) {
		event.WatchIDs = nil
	}
	return event, true
}

func resyncScope(event protocol.RuntimeEvent) (map[string]bool, map[protocol.RuntimeTopic]bool, bool) {
	watchIDs := make(map[string]bool, len(event.WatchIDs))
	for _, watchID := range event.WatchIDs {
		if watchID == "" || watchIDs[watchID] {
			return nil, nil, false
		}
		watchIDs[watchID] = true
	}
	knownTopics := protocol.RuntimeTopics()
	requested := make(map[protocol.RuntimeTopic]bool, len(event.Topics))
	for _, topic := range event.Topics {
		if !slices.Contains(knownTopics, topic) || requested[topic] {
			return nil, nil, false
		}
		requested[topic] = true
	}
	return watchIDs, requested, true
}

func (w *runtimeSubscription) heldWatchIDs(named map[string]bool) []string {
	held := make([]string, 0, len(named))
	for _, watchID := range w.watchIDs {
		if named[watchID] {
			held = append(held, watchID)
		}
	}
	return held
}

// Canonicalized before stallLocked: otherwise an invalid frame widens to a full
// resync when the queue has room but narrows to its topic when full.
func (w *runtimeSubscription) prepareLocked(event protocol.RuntimeEvent) (protocol.RuntimeEvent, bool) {
	next, ok := w.sequence.next()
	if !ok {
		w.exhaustLocked()
		return protocol.RuntimeEvent{}, false
	}
	event = cloneRuntimeEvent(event)
	clearEmptyRuntimeScopes(&event)
	event.Sequence = next.wire()
	if err := protocol.ValidateWireTree(event); err != nil {
		event = w.resyncEvent()
		event.Sequence = next.wire()
		if recoveryErr := protocol.ValidateWireTree(event); recoveryErr != nil {
			panic("delivery: invalid runtime resync invariant: " + recoveryErr.Error())
		}
	}
	return event, true
}

func (w *runtimeSubscription) offerPreparedLocked(event protocol.RuntimeEvent) bool {
	if w.sequenceExhausted() {
		return false
	}
	select {
	case w.events <- event:
		w.sequence = runtimeEventSequence(event.Sequence)
		return true
	default:
		return false
	}
}

func (w *runtimeSubscription) offerLocked(event protocol.RuntimeEvent) bool {
	prepared, ok := w.prepareLocked(event)
	return ok && w.offerPreparedLocked(prepared)
}

func (w *runtimeSubscription) exhaustLocked() {
	if w.sequenceExhausted() {
		return
	}
	w.stalledTopics = nil
	w.stalledWatchIDs = nil
	w.stalledAllWatches = false
	close(w.exhausted)
}

func (w *runtimeSubscription) sequenceExhausted() bool {
	select {
	case <-w.exhausted:
		return true
	default:
		return false
	}
}

func (w *runtimeSubscription) resyncEvent() protocol.RuntimeEvent {
	topics := make([]protocol.RuntimeTopic, 0, len(w.topics))
	for _, topic := range protocol.RuntimeTopics() {
		if w.topics[topic] {
			topics = append(topics, topic)
		}
	}
	return protocol.RuntimeEvent{Type: protocol.RuntimeResync, Topics: topics}
}

func clearEmptyRuntimeScopes(event *protocol.RuntimeEvent) {
	if len(event.Paths) == 0 {
		event.Paths = nil
	}
	if len(event.Names) == 0 {
		event.Names = nil
	}
	if len(event.Servers) == 0 {
		event.Servers = nil
	}
	if len(event.SessionIDs) == 0 {
		event.SessionIDs = nil
	}
	if len(event.RunIDs) == 0 {
		event.RunIDs = nil
	}
	if len(event.Topics) == 0 {
		event.Topics = nil
	}
	if len(event.WatchIDs) == 0 {
		event.WatchIDs = nil
	}
}

func (w *runtimeSubscription) stallLocked(event protocol.RuntimeEvent) {
	if w.sequenceExhausted() {
		return
	}
	if w.stalledTopics == nil {
		w.stalledTopics = make(map[protocol.RuntimeTopic]bool, len(protocol.RuntimeTopics()))
	}
	if event.Type == protocol.RuntimeResync {
		for _, topic := range event.Topics {
			w.stalledTopics[topic] = true
		}
		if slices.Contains(event.Topics, protocol.TopicFilesChanged) {
			w.stallFileScopeLocked(event.WatchIDs)
		}
		return
	}
	topic := protocol.RuntimeTopic(event.Type)
	w.stalledTopics[topic] = true
	if topic == protocol.TopicFilesChanged {
		if event.WatchID == "" {
			w.stallFileScopeLocked(nil)
		} else {
			w.stallFileScopeLocked([]string{event.WatchID})
		}
	}
}

// Broad invalidation dominates: once any missed file signal lacked a watch id, the
// resync must omit watchIds or the client would wrongly trust its other file
// projections.
func (w *runtimeSubscription) stallFileScopeLocked(ids []string) {
	if len(ids) == 0 {
		w.stalledAllWatches = true
		w.stalledWatchIDs = nil
		return
	}
	if w.stalledAllWatches {
		return
	}
	for _, id := range ids {
		if !slices.Contains(w.stalledWatchIDs, id) {
			w.stalledWatchIDs = append(w.stalledWatchIDs, id)
		}
	}
}

func (w *runtimeSubscription) flushStalledLocked() bool {
	if w.sequenceExhausted() {
		return false
	}
	if len(w.stalledTopics) == 0 {
		return true
	}
	topics := make([]protocol.RuntimeTopic, 0, len(w.stalledTopics))
	for _, topic := range protocol.RuntimeTopics() {
		if w.stalledTopics[topic] {
			topics = append(topics, topic)
		}
	}
	if !w.offerLocked(protocol.RuntimeEvent{
		Type: protocol.RuntimeResync, Topics: topics, WatchIDs: w.stalledWatchIDs,
	}) {
		return false
	}
	w.stalledTopics = nil
	w.stalledWatchIDs = nil
	w.stalledAllWatches = false
	return true
}

// Each subscription owns every mutable field: sharing the producer's slices would
// let a caller mutate one while a transport is still encoding it.
func cloneRuntimeEvent(event protocol.RuntimeEvent) protocol.RuntimeEvent {
	if event.Workspace != nil {
		event.Workspace = new(*event.Workspace)
	}
	event.Paths = slices.Clone(event.Paths)
	event.Names = slices.Clone(event.Names)
	event.Servers = slices.Clone(event.Servers)
	event.SessionIDs = slices.Clone(event.SessionIDs)
	event.RunIDs = slices.Clone(event.RunIDs)
	event.Topics = slices.Clone(event.Topics)
	event.WatchIDs = slices.Clone(event.WatchIDs)
	return event
}
