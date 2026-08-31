package runs

import (
	"errors"
	"iter"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/transcript"
	"github.com/Tangerg/flame/runtime/internal/testsupport/itemfixture"
)

const (
	testRunID     = "run_1"
	testSegmentID = "seg_1"
)

var testEpoch = testReplayEpoch("epoch_test")

func testJournal(t testing.TB) *journal {
	t.Helper()
	return mustNewJournal(t, testStreamScope(testEpoch, testRunID, testSegmentID), DefaultRetention())
}

func testStreamScope(epoch replayEpoch, runID, segmentID string) streamScope {
	scope, err := newStreamScope(epoch, runID, segmentID)
	if err != nil {
		panic(err)
	}
	return scope
}

func mustNewJournal(t testing.TB, scope streamScope, retention Retention) *journal {
	t.Helper()
	journal, err := newJournal(scope, retention)
	if err != nil {
		t.Fatalf("newJournal: %v", err)
	}
	return journal
}

func mustAppendJournal(t testing.TB, journal *journal, event Event) {
	t.Helper()
	if err := journal.append(event); err != nil {
		t.Fatalf("append journal event: %v", err)
	}
}

func mustCloseJournal(t testing.TB, journal *journal) {
	t.Helper()
	if err := journal.close(); err != nil {
		t.Fatalf("close journal: %v", err)
	}
}

func TestNewJournalRejectsInvalidResourceContracts(t *testing.T) {
	validScope := testStreamScope(testEpoch, testRunID, testSegmentID)
	if journal, err := newJournal(validScope, Retention{}); journal != nil || err == nil {
		t.Fatalf("newJournal with zero retention = (%v, %v), want nil/error", journal, err)
	}
	oversized := strings.Repeat("x", MaximumReplayCursorCharacters+1)
	if scope, err := newStreamScope(testEpoch, oversized, testSegmentID); err == nil || scope != (streamScope{}) {
		t.Fatalf("newStreamScope with oversized Run = (%v, %v), want zero/error", scope, err)
	}
}

func TestJournalRejectsSequenceExhaustionWithoutPublishingAPartialEvent(t *testing.T) {
	j := testJournal(t)
	j.head = ^uint64(0)
	j.headCursor = "previous"
	if err := j.append(ev(true)); !errors.Is(err, errReplaySequenceExhausted) {
		t.Fatalf("append at exhausted sequence err = %v, want sequence exhausted", err)
	}
	if j.head != ^uint64(0) || j.headCursor != "previous" || len(j.retained) != 0 {
		t.Fatalf("rejected append mutated journal: head=%d cursor=%q retained=%d", j.head, j.headCursor, len(j.retained))
	}
}

// ev builds a payload-only event. The journal assigns its position, so a test
// never states a sequence: stating one is exactly the mistake the journal now
// makes impossible.
func ev(replayable bool) Event {
	if replayable {
		return Event{RunID: testRunID, SegmentID: testSegmentID, Payload: SegmentStarted{}}
	}
	return Event{RunID: testRunID, SegmentID: testSegmentID, Payload: SegmentProgressed{}}
}

// sized builds a replayable event that retains n bytes of text, so a byte-budget
// test measures the real event shape rather than a fabricated charge.
func sized(n int) Event {
	return Event{
		RunID: testRunID, SegmentID: testSegmentID,
		Payload: ItemCompleted{Item: itemfixture.MustRestore(itemfixture.Input{
			ID:      "item_1",
			Content: []transcript.ContentBlock{{Kind: transcript.TextContent, Text: strings.Repeat("x", n)}},
		})},
	}
}

func cursorAt(t testing.TB, sequence uint64) string {
	t.Helper()
	cursor, err := encodeReplayCursor(replayPosition{
		epoch: testEpoch, runID: testRunResourceID(testRunID),
		segmentID: testSegmentResourceID(testSegmentID), sequence: sequence,
	})
	if err != nil {
		t.Fatalf("encode replay cursor: %v", err)
	}
	return cursor
}

func drain(seq iter.Seq[Event]) []uint64 {
	var got []uint64
	for e := range seq {
		got = append(got, e.Sequence)
	}
	return got
}

// A cursorless subscribe is TAIL-ONLY: history belongs to the transcript reads,
// and replaying it here would hand the client the same events twice.
func TestJournal_TailSkipsWhatWasAlreadyPublished(t *testing.T) {
	j := testJournal(t)
	mustAppendJournal(t, j, ev(true))
	mustAppendJournal(t, j, ev(true))

	attached := j.tail()
	defer attached.Cancel()
	mustAppendJournal(t, j, ev(true)) // the only event this subscription may see
	mustCloseJournal(t, j)

	if got := drain(attached.Events); len(got) != 1 || got[0] != 3 {
		t.Fatalf("tail delivered %v, want only the event appended after attaching", got)
	}
}

// The head is captured with the attach, under one lock. Without that, an event
// published in between would be neither in the ack nor in the stream.
func TestJournal_TailReportsTheHeadItAttachedAfter(t *testing.T) {
	j := testJournal(t)
	if head := j.tail().HeadCursor; head != "" {
		t.Fatalf("head of an empty stream = %q, want empty", head)
	}
	mustAppendJournal(t, j, ev(true))
	mustAppendJournal(t, j, ev(false)) // non-replayable events still take a position

	attached := j.tail()
	defer attached.Cancel()
	if attached.HeadCursor != cursorAt(t, 2) {
		t.Fatalf("head cursor does not name the last published position")
	}
}

func TestJournalTailFirstSnapshotConvergesAcrossTerminalBoundary(t *testing.T) {
	tests := []struct {
		name               string
		terminalBeforeTail bool
		terminalBeforeRead bool
	}{
		{name: "terminal committed before tail", terminalBeforeTail: true},
		{name: "terminal committed after tail before read", terminalBeforeRead: true},
		{name: "terminal committed after read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			journal := testJournal(t)
			durableTerminal := false
			publishTerminal := func() {
				durableTerminal = true
				mustAppendJournal(t, journal, Event{
					RunID: testRunID, SegmentID: testSegmentID,
					Payload: SegmentFinished{},
				})
			}
			if tt.terminalBeforeTail {
				publishTerminal()
			}
			attached := journal.tail()
			defer attached.Cancel()
			if tt.terminalBeforeRead {
				publishTerminal()
			}

			foldedTerminal := durableTerminal
			if !tt.terminalBeforeTail && !tt.terminalBeforeRead {
				publishTerminal()
			}
			mustCloseJournal(t, journal)
			for event := range attached.Events {
				if _, ok := event.Payload.(SegmentFinished); ok {
					foldedTerminal = true
				}
			}
			if !foldedTerminal {
				t.Fatal("tail-first snapshot lost the terminal boundary")
			}
		})
	}
}

func TestJournal_ReplayServesWhatFollowsTheCursorThenTails(t *testing.T) {
	j := testJournal(t)
	for range 3 {
		mustAppendJournal(t, j, ev(true))
	}

	attached, err := j.replay(cursorAt(t, 2))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer attached.Cancel()
	mustAppendJournal(t, j, ev(true))
	mustCloseJournal(t, j)

	if got := drain(attached.Events); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("replay delivered %v, want [3 4] (backlog then live)", got)
	}
}

// Ephemeral events take a position but are never retained, so a cursor pointing
// at one still resumes correctly — everything replayable after it is served.
func TestJournal_ReplayFromAnEphemeralPositionIsExact(t *testing.T) {
	j := testJournal(t)
	mustAppendJournal(t, j, ev(true))  // 1
	mustAppendJournal(t, j, ev(false)) // 2, not retained
	mustAppendJournal(t, j, ev(true))  // 3

	attached, err := j.replay(cursorAt(t, 2))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer attached.Cancel()
	mustCloseJournal(t, j)
	if got := drain(attached.Events); len(got) != 1 || got[0] != 3 {
		t.Fatalf("replay delivered %v, want [3]", got)
	}
}

func TestJournal_ReplayRefusesCursorsItCannotServe(t *testing.T) {
	other := func(position replayPosition) string {
		cursor, err := encodeReplayCursor(position)
		if err != nil {
			t.Fatalf("encode foreign replay cursor: %v", err)
		}
		return cursor
	}
	for name, test := range map[string]struct {
		cursor string
		want   error
	}{
		"damaged": {cursor: "!!!", want: ErrReplayCursorInvalid},
		"another run": {cursor: other(replayPosition{
			epoch: testEpoch, runID: testRunResourceID("run_other"),
			segmentID: testSegmentResourceID(testSegmentID), sequence: 1,
		}), want: ErrReplayCursorInvalid},
		// The previous segment of the SAME run: the case a resume creates, and the one
		// a client is most likely to hold.
		"another segment": {cursor: other(replayPosition{
			epoch: testEpoch, runID: testRunResourceID(testRunID),
			segmentID: testSegmentResourceID("seg_previous"), sequence: 1,
		}), want: ErrReplayCursorInvalid},
		"ahead of the head": {cursor: cursorAt(t, 99), want: ErrReplayCursorInvalid},
		"another process": {cursor: other(replayPosition{
			epoch: testReplayEpoch("epoch_previous"), runID: testRunResourceID(testRunID),
			segmentID: testSegmentResourceID(testSegmentID), sequence: 1,
		}), want: ErrReplayUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			j := testJournal(t)
			mustAppendJournal(t, j, ev(true))
			if _, err := j.replay(test.cursor); !errors.Is(err, test.want) {
				t.Fatalf("replay err = %v, want %v", err, test.want)
			}
		})
	}
}

// A cursor from a process that has restarted is unavailable rather than invalid,
// even when its run and segment are unknown here: the client did nothing wrong,
// and its remedy is a cold recovery rather than a corrected request.
func TestJournal_ForeignEpochOutranksAForeignScope(t *testing.T) {
	j := testJournal(t)
	mustAppendJournal(t, j, ev(true))
	stale, err := encodeReplayCursor(replayPosition{
		epoch: testReplayEpoch("epoch_previous"), runID: testRunResourceID("run_other"),
		segmentID: testSegmentResourceID("seg_other"), sequence: 1,
	})
	if err != nil {
		t.Fatalf("encode stale replay cursor: %v", err)
	}
	if _, err := j.replay(stale); !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("replay err = %v, want ErrReplayUnavailable", err)
	}
}

func TestJournal_EvictionBoundsTheWindowByCount(t *testing.T) {
	j := mustNewJournal(t, testStreamScope(testEpoch, testRunID, testSegmentID),
		Retention{MaxEvents: 2, MaxBytes: DefaultRetention().MaxBytes})
	for range 4 {
		mustAppendJournal(t, j, ev(true))
	}

	// Events 1 and 2 are gone, so a cursor before them cannot be served — that is
	// the difference between "nothing new for you" and "you have missed something".
	if _, err := j.replay(cursorAt(t, 1)); !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("evicted cursor err = %v, want ErrReplayUnavailable", err)
	}
	attached, err := j.replay(cursorAt(t, 2))
	if err != nil {
		t.Fatalf("cursor at the eviction boundary: %v", err)
	}
	defer attached.Cancel()
	mustCloseJournal(t, j)
	if got := drain(attached.Events); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Fatalf("boundary replay = %v, want [3 4]", got)
	}
}

// The count budget alone does not bound memory: a handful of events can hold a
// multi-megabyte tool result each.
func TestJournal_EvictionBoundsTheWindowByBytes(t *testing.T) {
	const payload = 4096
	j := mustNewJournal(t, testStreamScope(testEpoch, testRunID, testSegmentID),
		Retention{MaxEvents: 1024, MaxBytes: payload * 2})
	for range 4 {
		mustAppendJournal(t, j, sized(payload))
	}

	j.mu.Lock()
	retained, bytes := len(j.retained), j.retainedBytes
	j.mu.Unlock()
	if retained >= 4 {
		t.Fatalf("retained %d events, want the byte budget to have evicted some", retained)
	}
	if bytes > payload*2 {
		t.Fatalf("retained bytes = %d, want at most %d", bytes, payload*2)
	}
	if _, err := j.replay(cursorAt(t, 1)); !errors.Is(err, ErrReplayUnavailable) {
		t.Fatalf("evicted cursor err = %v, want ErrReplayUnavailable", err)
	}
}

// TestJournalReplayableLosslessLiveLossyUnderOverflow locks the drop policy: a
// subscriber flooded past its buffering without draining still receives every
// replayable event in order, while live-only events become a lossy but
// still-ordered subset. The surviving live count is deliberately NOT asserted —
// buffer size is an implementation detail; "replayable never drops, live-only
// may" is the contract.
func TestJournalReplayableLosslessLiveLossyUnderOverflow(t *testing.T) {
	j := testJournal(t)
	attached := j.tail()
	defer attached.Cancel()

	const liveTotal = liveHeadroom * 4
	for i := 1; i <= liveTotal; i++ {
		if i == 1 || i == liveTotal/2 {
			mustAppendJournal(t, j, ev(true))
		}
		mustAppendJournal(t, j, ev(false))
	}
	mustAppendJournal(t, j, ev(true))
	mustCloseJournal(t, j)

	var gotReplayable []uint64
	deliveredLive := 0
	for e := range attached.Events {
		if e.Replayable() {
			gotReplayable = append(gotReplayable, e.Sequence)
			continue
		}
		deliveredLive++
	}
	wantReplayable := []uint64{1, uint64(liveTotal/2 + 1), uint64(liveTotal + 3)}
	if len(gotReplayable) != len(wantReplayable) {
		t.Fatalf("replayable delivered = %v, want %v (replayable must be lossless)", gotReplayable, wantReplayable)
	}
	for i := range wantReplayable {
		if gotReplayable[i] != wantReplayable[i] {
			t.Fatalf("replayable[%d] = %d, want %d (order must hold)", i, gotReplayable[i], wantReplayable[i])
		}
	}
	if deliveredLive >= liveTotal {
		t.Fatalf("live-only delivered = %d, want < %d (overflow must drop live-only)", deliveredLive, liveTotal)
	}
}

// A replayable event is never dropped, so a consumer that stops draining is
// disconnected instead. Serving it a stream with a hole would leave it folding a
// state it could not tell was wrong.
func TestJournal_StalledAuthoritativeConsumerIsDisconnected(t *testing.T) {
	j := mustNewJournal(t, testStreamScope(testEpoch, testRunID, testSegmentID),
		Retention{MaxEvents: 3, MaxBytes: DefaultRetention().MaxBytes})
	attached := j.tail()
	defer attached.Cancel()

	for range 5 {
		mustAppendJournal(t, j, ev(true))
	}
	got := drain(attached.Events)
	if len(got) >= 5 {
		t.Fatalf("stalled subscriber received %v, want the stream to have ended early", got)
	}
	// The run keeps going: a slow client must never stall the agent.
	mustAppendJournal(t, j, ev(true))
	j.mu.Lock()
	head := j.head
	j.mu.Unlock()
	if head != 6 {
		t.Fatalf("stream head = %d, want 6 (the run must keep publishing)", head)
	}
}

// Disconnecting a subscriber is also a journal-ownership transition. A caller
// may not have started ranging yet (for example while its transport is still
// writing the stream acknowledgement), so relying on the sequence's deferred
// Cancel would retain the aborted subscriber for the rest of a long-running
// Segment and visit it on every later publication.
func TestJournal_OverflowDetachesBeforeTheConsumerStarts(t *testing.T) {
	j := mustNewJournal(t, testStreamScope(testEpoch, testRunID, testSegmentID),
		Retention{MaxEvents: 2, MaxBytes: DefaultRetention().MaxBytes})
	attached := j.tail()
	defer attached.Cancel()

	for range 3 {
		mustAppendJournal(t, j, ev(true))
	}

	j.mu.Lock()
	subscribers := len(j.subs)
	j.mu.Unlock()
	if subscribers != 0 {
		t.Fatalf("subscribers after authoritative overflow = %d, want 0", subscribers)
	}
	if got := drain(attached.Events); len(got) != 0 {
		t.Fatalf("aborted subscriber delivered queued events: %v", got)
	}
}

// TestJournalCancelUnblocksWaitingSubscriber guards the external cancellation
// contract: a consumer blocked inside the source cannot stop its own range.
func TestJournalCancelUnblocksWaitingSubscriber(t *testing.T) {
	j := testJournal(t)
	attached := j.tail()

	done := make(chan struct{})
	go func() {
		consumeEvents(attached.Events) // no events will ever arrive; blocks in the source
		close(done)
	}()

	attached.Cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not unblock a subscriber waiting for its next event")
	}
}

func TestJournal_LiveOnlyIsNeverReplayed(t *testing.T) {
	j := testJournal(t)
	live := j.tail()
	defer live.Cancel()
	next, stop := iter.Pull(live.Events)
	defer stop()
	mustAppendJournal(t, j, ev(true))
	mustAppendJournal(t, j, ev(false))
	if got, _ := next(); got.Sequence != 1 {
		t.Fatal("live subscriber missed the replayable event")
	}
	if got, _ := next(); got.Sequence != 2 {
		t.Fatal("live subscriber missed the non-replayable event")
	}

	late, err := j.replay(cursorAt(t, 1))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer late.Cancel()
	mustCloseJournal(t, j)
	if got := drain(late.Events); len(got) != 0 {
		t.Fatalf("replay = %v, want no non-replayable events", got)
	}
}

func TestJournal_FanOutN(t *testing.T) {
	j := testJournal(t)
	a := j.tail()
	defer a.Cancel()
	nextA, stopA := iter.Pull(a.Events)
	defer stopA()
	b := j.tail()
	defer b.Cancel()
	nextB, stopB := iter.Pull(b.Events)
	defer stopB()

	mustAppendJournal(t, j, ev(true))
	if got, _ := nextA(); got.Sequence != 1 {
		t.Fatal("subscriber a must receive the event")
	}
	if got, _ := nextB(); got.Sequence != 1 {
		t.Fatal("subscriber b must receive the event")
	}
}

func TestJournal_CloseEndsStream(t *testing.T) {
	j := testJournal(t)
	mustAppendJournal(t, j, ev(true))
	attached, err := j.replay(cursorAt(t, 1))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	defer attached.Cancel()
	next, stop := iter.Pull(attached.Events)
	defer stop()

	mustAppendJournal(t, j, ev(true))
	if got, _ := next(); got.Sequence != 2 {
		t.Fatal("live event was not delivered")
	}
	mustCloseJournal(t, j)
	if _, ok := next(); ok {
		t.Fatal("stream must end on journal Close")
	}

	// A subscribe that arrives after the segment ended still gets what it missed,
	// then ends — the window outlives the pump by the width of that race.
	post, err := j.replay(cursorAt(t, 1))
	if err != nil {
		t.Fatalf("post-close replay: %v", err)
	}
	if got := drain(post.Events); len(got) != 1 || got[0] != 2 {
		t.Fatalf("post-close replay = %v, want [2]", got)
	}
}

func TestJournal_CancelDetaches(t *testing.T) {
	j := testJournal(t)
	attached := j.tail()
	attached.Cancel()
	if got := drain(attached.Events); len(got) != 0 {
		t.Fatalf("cancel must end the stream, got %v", got)
	}
	mustAppendJournal(t, j, ev(true)) // must not panic (sub gone)
	mustCloseJournal(t, j)            // must not double-anything
}

func TestJournal_EarlyRangeStopDetaches(t *testing.T) {
	j := testJournal(t)
	attached := j.tail()
	mustAppendJournal(t, j, ev(true))

	for range attached.Events {
		break
	}

	j.mu.Lock()
	subscribers := len(j.subs)
	j.mu.Unlock()
	if subscribers != 0 {
		t.Fatalf("subscribers after range stop = %d, want 0", subscribers)
	}

	mustAppendJournal(t, j, ev(true)) // must not enqueue into an abandoned subscriber
	mustCloseJournal(t, j)
}

func TestJournalSubscriber_ReusesRoutineQueueAndReleasesBursts(t *testing.T) {
	subscriber := newJournalSubscriber(nil, DefaultRetention())
	subscriber.enqueue(ev(false), 0)
	if _, ok := subscriber.next(); !ok {
		t.Fatal("routine event was not delivered")
	}
	routineCapacity := cap(subscriber.queue)
	if routineCapacity == 0 {
		t.Fatal("routine queue capacity was not retained")
	}

	subscriber.enqueue(ev(false), 0)
	if _, ok := subscriber.next(); !ok {
		t.Fatal("reused queue event was not delivered")
	}
	if got := cap(subscriber.queue); got != routineCapacity {
		t.Fatalf("routine queue capacity = %d, want reused capacity %d", got, routineCapacity)
	}

	for range liveHeadroom * 2 {
		subscriber.enqueue(ev(true), 1)
	}
	for i := range liveHeadroom * 2 {
		if _, ok := subscriber.next(); !ok {
			t.Fatalf("replayable burst ended at event %d", i)
		}
	}
	if subscriber.queue != nil {
		t.Fatalf("oversized drained queue retained capacity %d", cap(subscriber.queue))
	}
}

func TestJournalSubscriber_AbortReleasesQueuedEvents(t *testing.T) {
	subscriber := newJournalSubscriber(nil, DefaultRetention())
	subscriber.enqueue(ev(true), 1)
	subscriber.abort()

	if subscriber.queue != nil {
		t.Fatalf("aborted queue retained capacity %d", cap(subscriber.queue))
	}
	if _, ok := subscriber.next(); ok {
		t.Fatal("aborted subscriber delivered a queued event")
	}
}

// A backlog within the window is lossless however far behind the consumer is.
func TestJournal_ReplayableBacklogWithinTheWindowIsLossless(t *testing.T) {
	j := testJournal(t)
	attached := j.tail()
	defer attached.Cancel()
	const total = liveHeadroom*3 + 17
	for range total {
		mustAppendJournal(t, j, ev(true))
	}
	mustCloseJournal(t, j)

	got := drain(attached.Events)
	if len(got) != total {
		t.Fatalf("replayable events = %d, want %d", len(got), total)
	}
	for i, sequence := range got {
		if want := uint64(i + 1); sequence != want {
			t.Fatalf("event[%d] = %d, want %d", i, sequence, want)
		}
	}
}

func TestJournalConcurrentAppendCloseAndCancel(t *testing.T) {
	j := testJournal(t)
	attached := j.tail()
	start := make(chan struct{})
	var wg sync.WaitGroup
	var appendErr, closeErr error
	wg.Go(func() {
		<-start
		for range liveHeadroom * 2 {
			if appendErr = j.append(ev(true)); appendErr != nil {
				return
			}
		}
	})
	wg.Go(func() {
		<-start
		closeErr = j.close()
	})
	wg.Go(func() {
		<-start
		attached.Cancel()
	})
	close(start)
	wg.Wait()
	if appendErr != nil {
		t.Fatalf("append journal event: %v", appendErr)
	}
	if closeErr != nil {
		t.Fatalf("close journal: %v", closeErr)
	}
	consumeEvents(attached.Events) // drain whatever survived the race; must terminate
}

// BenchmarkJournalAppendDrain records the steady-state per-event append→deliver
// cost through one subscriber. Live-only events avoid retention.
func BenchmarkJournalAppendDrain(b *testing.B) {
	j := testJournal(b)
	attached := j.tail()
	defer attached.Cancel()
	next, stop := iter.Pull(attached.Events)
	defer stop()
	e := ev(false)
	for b.Loop() {
		if err := j.append(e); err != nil {
			b.Fatal(err)
		}
		next()
	}
}
