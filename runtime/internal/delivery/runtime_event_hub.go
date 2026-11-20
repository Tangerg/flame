package delivery

import (
	"errors"
	"slices"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

// errSubscriptionAdmissionsClosed reports that a runtime subscription could not
// start because the Handler has stopped admitting new streams.
var errSubscriptionAdmissionsClosed = errors.New("delivery: runtime subscriptions are closed")

// runtimeSubscriptionQueueCapacity absorbs short UI/refetch bursts without
// back-pressuring publishers. Sustained lag does not consume more memory: the
// subscription folds excess signals into one scoped resync.
const runtimeSubscriptionQueueCapacity = 64

// runtimeEventHub fans runtime change signals out to the live runtime.subscribe
// streams. It is the non-run, ephemeral counterpart to the per-run hubs:
// coalescing rather than back-pressuring the publisher (a change signal carries no
// truth of its own, so several of them collapse into "re-read these topics"),
// connection-scoped (no retained replay), shared by the whole app.
//
// It never drops an invalidation and never skips a sequence number. A slow
// subscriber's undelivered signals fold into one pending resync naming exactly the
// topics involved, which goes out as soon as its queue has room — either behind the
// next signal or when the consumer drains one. The two are not interchangeable: a
// gap tells a client only that something was lost, while a resync tells it what to
// read, and inferring loss from a missing number means a client that never notices
// on a quiet stream.
type runtimeEventHub struct {
	mu            sync.Mutex
	subscriptions map[*runtimeSubscription]struct{}
	closed        bool
}

func newRuntimeEventHub() *runtimeEventHub {
	return &runtimeEventHub{subscriptions: make(map[*runtimeSubscription]struct{})}
}

type runtimeSubscription struct {
	events chan protocol.RuntimeEvent
	// exhausted closes exactly once when this connection has used every sequence
	// identity JSON clients can represent exactly. The stream drains frames already
	// accepted by events, then its owner stops watchers and unregisters it.
	exhausted chan struct{}
	// topics is what THIS subscription asked for. The hub broadcasts every signal it
	// receives; a subscription only sees the ones it can fold. A resync is narrowed
	// to this same set before delivery: a client cannot re-read a topic it never
	// declared.
	topics map[protocol.RuntimeTopic]bool
	// watchIDs is the stable, client-declared file-watch scope of THIS stream.
	// Watch identifiers are connection-local: a targeted file invalidation from
	// another subscription must never cross into this one merely because both
	// clients subscribed to files.changed.
	watchIDs []string
	// sequence is assigned when a frame is handed to the queue, never when one is
	// produced or filtered — so the numbers a subscriber sees are consecutive and a
	// gap can only mean the transport lost a frame.
	sequence runtimeEventSequence
	// stalledTopics / stalledWatchIDs accumulate the scope of signals a full queue
	// could not take. They become one resync; until it is delivered every further
	// signal folds into them, because a client must not receive a later invalidation
	// ahead of the notice that it missed earlier ones.
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

// register adds a caller-owned channel to the broadcast fan-out and returns an
// idempotent unregister. It does NOT close the channel — the owner does, after
// it has stopped every other writer (the file watcher), so a late broadcast
// can't send on a closed channel.
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

// closeAdmissions linearizes Handler retirement with workspace subscription
// registration. Existing request-owned streams keep running until their own
// contexts end, but once this returns no racing check-then-register path can
// create another subscription.
func (w *runtimeEventHub) closeAdmissions() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
}

// publish fans event to every subscriber, folding it into a pending resync for any
// whose buffer is full (see the type doc).
func (w *runtimeEventHub) publish(event protocol.RuntimeEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for subscription := range w.subscriptions {
		w.sendLocked(subscription, event)
	}
}

// drained gives a stalled subscription its chance to catch up: the consumer just
// took a frame, so the queue has room the publisher did not have. Without this a
// pending resync would wait for the next unrelated signal, which on a quiet stream
// is never — the loss would be silent, which is the one outcome this hub forbids.
func (w *runtimeEventHub) drained(subscription *runtimeSubscription) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, registered := w.subscriptions[subscription]; registered {
		subscription.flushStalledLocked()
	}
}

// publishTo sends a subscription-local event through the same serialization
// point as broadcasts. This keeps each subscriber's sequence strictly ordered
// even when its git watcher races a global workspace event.
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
			// A watch id is meaningful only inside the stream that declared it.
			// Unscoped tool-write notices have no watch id and still fan out to
			// every files.changed subscriber.
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

// scopeResync intersects a valid resync with what this subscription declared,
// in protocol declaration order. Invalid scope is intentionally left intact:
// offerLocked's complete wire validation then widens it to this subscription's
// full resync instead of guessing what a malformed producer meant.
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
		// watchIds only narrows files.changed. This cross-field contradiction cannot
		// be scoped without guessing which part the producer meant, so fail closed to
		// the complete set of facts held by this subscription.
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

// resyncScope reads the watches and topics a resync names. A repeated, empty
// or unknown entry makes the scope malformed, and the event passes unscoped.
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

// heldWatchIDs keeps the named watches this subscription holds, in its order.
func (w *runtimeSubscription) heldWatchIDs(named map[string]bool) []string {
	held := make([]string, 0, len(named))
	for _, watchID := range w.watchIDs {
		if named[watchID] {
			held = append(held, watchID)
		}
	}
	return held
}

// prepareLocked gives a signal one canonical, validated meaning before either
// queueing or coalescing it. Doing this before stallLocked is essential: otherwise
// an invalid producer frame is widened to a full resync when the queue has room but
// narrowed to its superficial topic when the queue is full.
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

// offerPreparedLocked hands a canonical event to the queue, advancing the
// subscription sequence only when it fits.
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

// offerLocked is the preparation boundary used by synthetic recovery events.
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

// clearEmptyRuntimeScopes canonicalizes an output value before validation.
// encoding/json omits every empty `omitempty` slice, so nil is the one in-memory
// spelling of absence. Required scopes (files paths and resync topics) remain
// absent and fail their variant rule; optional empty scopes simply disappear.
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

// stallLocked folds an undeliverable signal's scope into the pending resync. A
// resync being stalled contributes its own scope, so a coalesced resync never
// narrows what an earlier one had already widened.
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

// stallFileScopeLocked unions targeted watches, but broad invalidation dominates:
// once any missed file signal had no watch id, the eventual resync must omit
// watchIds too. Retaining an earlier narrow list would falsely tell the client
// that its other file projections are still current.
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

// flushStalledLocked delivers the pending resync if there is one. Reports whether
// the subscription is caught up — false means the queue is still full and the
// caller's own signal has to fold in too.
func (w *runtimeSubscription) flushStalledLocked() bool {
	if w.sequenceExhausted() {
		return false
	}
	if len(w.stalledTopics) == 0 {
		return true
	}
	// Declaration order, not map order: the topics a client is told to re-read are
	// part of the wire, and a set that reshuffles per delivery is a fixture nobody
	// can pin down.
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

// cloneRuntimeEvent gives each subscription sole ownership of every mutable field.
// The hub sends asynchronously-consumed values; sharing the producer's slices would
// let a caller reuse one while a transport is still encoding the event, and sharing
// between subscriptions would let one in-process consumer corrupt another's frame.
//
// Every reference-typed field is copied, pointers included.
// TestRuntimeEventCloneOwnsEveryReferenceField fails when a new one is added
// without a copy here, because a comment describing the shape of this struct is
// exactly the thing that goes stale when the struct grows.
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
