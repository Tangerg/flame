package runs

import (
	"errors"
	"fmt"
	"iter"
	"math"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

const liveHeadroom = 256

type Retention struct {
	MaxEvents int
	MaxBytes  int
}

const (
	defaultRetainedEvents = 2048
	defaultRetainedBytes  = 16 << 20
)

func DefaultRetention() Retention {
	return Retention{MaxEvents: defaultRetainedEvents, MaxBytes: defaultRetainedBytes}
}

var (
	ErrReplayCursorInvalid = errors.New("runs: replay cursor does not address this stream")
	ErrReplayUnavailable   = errors.New("runs: replay window no longer reaches this cursor")
)

type streamScope struct {
	Epoch     replayEpoch
	RunID     identity.RunID
	SegmentID identity.SegmentID
}

func newStreamScope(epoch replayEpoch, runID identity.RunID, segmentID identity.SegmentID) (streamScope, error) {
	if err := epoch.Validate(); err != nil {
		return streamScope{}, err
	}
	parsedRunID := runID
	if err := runID.Validate(); err != nil {
		return streamScope{}, err
	}
	parsedSegmentID := segmentID
	if err := segmentID.Validate(); err != nil {
		return streamScope{}, err
	}
	return streamScope{Epoch: epoch, RunID: parsedRunID, SegmentID: parsedSegmentID}, nil
}

type subscription struct {
	Events     iter.Seq[Event]
	Cancel     func()
	HeadCursor string
}

type journal struct {
	mu             sync.Mutex
	scope          streamScope
	retention      Retention
	head           uint64
	headCursor     string
	retained       []chargedEvent
	retainedBytes  int
	evictedThrough uint64
	subs           map[*journalSubscriber]struct{}
	closeEvent     *chargedEvent
	closed         bool
}

func newJournal(scope streamScope, retention Retention) (*journal, error) {
	if retention.MaxEvents <= 0 || retention.MaxBytes <= 0 {
		return nil, errors.New("runs: journal retention budgets must be positive")
	}
	if err := validateReplayScope(scope); err != nil {
		return nil, fmt.Errorf("runs: journal replay scope: %w", err)
	}
	return &journal{scope: scope, retention: retention, subs: map[*journalSubscriber]struct{}{}}, nil
}

// The position is assigned under the fan-out lock so sequence, publication and
// replay order are one order.
func (j *journal) append(ev Event) error {
	charged := chargeJournalEvent(ev)
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return nil
	}
	return j.appendLocked(charged)
}

func (j *journal) deferCloseEvent(ev Event) {
	charged := chargeJournalEvent(ev)
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.closed {
		return
	}
	if j.closeEvent != nil {
		panic("runs: journal received more than one close event")
	}
	j.closeEvent = &charged
}

func chargeJournalEvent(ev Event) chargedEvent {
	size := 0
	if ev.Replayable() {
		size = retainedBytes(ev)
	}
	return chargedEvent{event: ev, bytes: size}
}

func (j *journal) appendLocked(charged chargedEvent) error {
	if j.head == math.MaxUint64 {
		return errReplaySequenceExhausted
	}
	ev := charged.event
	nextSequence := j.head + 1
	nextCursor, err := encodeReplayCursor(replayPosition{
		epoch: j.scope.Epoch, runID: j.scope.RunID,
		segmentID: j.scope.SegmentID, sequence: nextSequence,
	})
	if err != nil {
		return err
	}
	j.head = nextSequence
	j.headCursor = nextCursor
	ev.Sequence = nextSequence
	if ev.Replayable() {
		charged.bytes += len(nextCursor) - len(ev.Cursor)
	}
	ev.Cursor = nextCursor
	if ev.Replayable() {
		j.retained = append(j.retained, chargedEvent{event: ev, bytes: charged.bytes})
		j.retainedBytes += charged.bytes
		j.evictLocked()
	}
	for subscriber := range j.subs {
		if !subscriber.enqueue(ev, charged.bytes) {
			// Overflow is a disconnection even when the consumer has not started
			// ranging and therefore cannot run the sequence's deferred Cancel.
			// The journal owns this registry, so retire the subscriber here under
			// the same lock that serialized the publication which disconnected it.
			delete(j.subs, subscriber)
		}
	}
	return nil
}

func (j *journal) evictLocked() {
	for len(j.retained) > 0 &&
		(len(j.retained) > j.retention.MaxEvents || j.retainedBytes > j.retention.MaxBytes) {
		oldest := j.retained[0]
		j.retained[0] = chargedEvent{}
		j.retained = j.retained[1:]
		j.retainedBytes -= oldest.bytes
		j.evictedThrough = oldest.event.Sequence
	}
}

func (j *journal) close() error {
	j.mu.Lock()
	if j.closed {
		j.mu.Unlock()
		return nil
	}
	var closeErr error
	if j.closeEvent != nil {
		closeErr = j.appendLocked(*j.closeEvent)
		j.closeEvent = nil
	}
	j.closed = true
	subscribers := make([]*journalSubscriber, 0, len(j.subs))
	for subscriber := range j.subs {
		subscribers = append(subscribers, subscriber)
		delete(j.subs, subscriber)
	}
	j.mu.Unlock()

	for _, subscriber := range subscribers {
		subscriber.finish()
	}
	return closeErr
}

// Capturing the head and attaching happen under one lock so nothing is published
// in between.
func (j *journal) tail() subscription {
	j.mu.Lock()
	head := j.headCursorLocked()
	return j.attachLocked(nil, head)
}

func (j *journal) attach(token string) (subscription, error) {
	if token == "" {
		return j.tail(), nil
	}
	return j.replay(token)
}

func (j *journal) replay(token string) (subscription, error) {
	from, err := decodeReplayCursor(token)
	if err != nil {
		return subscription{}, fmt.Errorf("%w: %w", ErrReplayCursorInvalid, err)
	}
	j.mu.Lock()
	if from.epoch != j.scope.Epoch {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor was minted by another runtime process", ErrReplayUnavailable)
	}
	if from.runID != j.scope.RunID || from.segmentID != j.scope.SegmentID {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor belongs to run %s segment %s",
			ErrReplayCursorInvalid, from.runID, from.segmentID)
	}
	if from.sequence > j.head {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor is ahead of the stream", ErrReplayCursorInvalid)
	}
	if from.sequence < j.evictedThrough {
		j.mu.Unlock()
		return subscription{}, fmt.Errorf("%w: cursor precedes the retained window", ErrReplayUnavailable)
	}
	backlog := make([]chargedEvent, 0, len(j.retained))
	for _, retained := range j.retained {
		if retained.event.Sequence > from.sequence {
			backlog = append(backlog, retained)
		}
	}
	return j.attachLocked(backlog, j.headCursorLocked()), nil
}

func (j *journal) headCursorLocked() string {
	return j.headCursor
}

func (j *journal) attachLocked(backlog []chargedEvent, head string) subscription {
	if j.closed {
		j.mu.Unlock()
		subscriber := newJournalSubscriber(backlog, j.retention)
		subscriber.finish()
		return subscription{Events: subscriber.events(), Cancel: func() {}, HeadCursor: head}
	}

	subscriber := newJournalSubscriber(backlog, j.retention)
	j.subs[subscriber] = struct{}{}
	j.mu.Unlock()

	var cancelOnce sync.Once
	cancel := func() {
		cancelOnce.Do(func() {
			j.mu.Lock()
			delete(j.subs, subscriber)
			j.mu.Unlock()
			subscriber.abort()
		})
	}
	events := subscriber.events()
	return subscription{
		Events: func(yield func(Event) bool) {
			defer cancel()
			events(yield)
		},
		Cancel:     cancel,
		HeadCursor: head,
	}
}

type chargedEvent struct {
	event Event
	bytes int
}

type journalSubscriber struct {
	mu               sync.Mutex
	ready            *sync.Cond
	retention        Retention
	queue            []chargedEvent
	head             int
	queuedLive       int
	queuedReplayable int
	queuedBytes      int
	phase            subscriberPhase
}

type subscriberPhase uint8

const (
	subscriberOpen subscriberPhase = iota
	subscriberFinishing
	subscriberAborted
)

func newJournalSubscriber(backlog []chargedEvent, retention Retention) *journalSubscriber {
	subscriber := &journalSubscriber{
		retention: retention,
		queue:     append(make([]chargedEvent, 0, len(backlog)), backlog...),
	}
	subscriber.ready = sync.NewCond(&subscriber.mu)
	subscriber.queuedReplayable = len(backlog)
	for _, charged := range backlog {
		subscriber.queuedBytes += charged.bytes
	}
	return subscriber
}

// A replayable event is never dropped: a stream missing one would fold into a
// wrong state undetectably, so authoritative overflow disconnects instead.
func (j *journalSubscriber) enqueue(ev Event, size int) bool {
	j.mu.Lock()
	if j.phase != subscriberOpen {
		j.mu.Unlock()
		return false
	}
	if !ev.Replayable() {
		if j.queuedLive >= liveHeadroom {
			j.mu.Unlock()
			return true
		}
		j.queuedLive++
	} else {
		if j.queuedReplayable >= j.retention.MaxEvents || j.queuedBytes+size > j.retention.MaxBytes {
			j.mu.Unlock()
			j.abort()
			return false
		}
		j.queuedReplayable++
		j.queuedBytes += size
	}
	if j.head > 0 && len(j.queue) == cap(j.queue) {
		remaining := copy(j.queue, j.queue[j.head:])
		clear(j.queue[remaining:])
		j.queue = j.queue[:remaining]
		j.head = 0
	}
	j.queue = append(j.queue, chargedEvent{event: ev, bytes: size})
	j.ready.Signal()
	j.mu.Unlock()
	return true
}

func (j *journalSubscriber) finish() {
	j.mu.Lock()
	if j.phase == subscriberOpen {
		j.phase = subscriberFinishing
	}
	j.ready.Broadcast()
	j.mu.Unlock()
}

func (j *journalSubscriber) abort() {
	j.mu.Lock()
	j.phase = subscriberAborted
	clear(j.queue[j.head:])
	j.queue = nil
	j.head = 0
	j.queuedLive = 0
	j.queuedReplayable = 0
	j.queuedBytes = 0
	j.ready.Broadcast()
	j.mu.Unlock()
}

func (j *journalSubscriber) events() iter.Seq[Event] {
	return func(yield func(Event) bool) {
		for {
			ev, ok := j.next()
			if !ok {
				return
			}
			if !yield(ev) {
				return
			}
		}
	}
}

func (j *journalSubscriber) next() (Event, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for j.head == len(j.queue) && j.phase == subscriberOpen {
		j.ready.Wait()
	}
	if j.phase == subscriberAborted || (j.head == len(j.queue) && j.phase == subscriberFinishing) {
		return Event{}, false
	}
	queued := j.queue[j.head]
	j.queue[j.head] = chargedEvent{}
	j.head++
	if j.head == len(j.queue) {
		if cap(j.queue) > liveHeadroom {
			j.queue = nil
		} else {
			j.queue = j.queue[:0]
		}
		j.head = 0
	}
	if !queued.event.Replayable() {
		j.queuedLive--
	} else {
		j.queuedReplayable--
		j.queuedBytes -= queued.bytes
	}
	return queued.event, true
}
