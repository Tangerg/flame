package queue

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

// enqueue mirrors the authoring transaction: the command identity is allocated
// before the queue sees it, exactly as the terminal allocates it.
func enqueue(q *Queue, sessionID string, message prompt.Message) (Entry, error) {
	return q.EnqueueCommand(
		nextCommandID(q), sessionID, message,
		prompt.RunOptions{},
	)
}

func TestEntryIdentityRejectsZeroAndAllocationOverflow(t *testing.T) {
	t.Parallel()
	if err := (EntryID{}).Validate(); err == nil {
		t.Fatal("zero queue entry identity was constructed")
	}
	if err := (EntryID{}).Validate(); err == nil {
		t.Fatal("zero queue entry identity was validated")
	}
	prompts := New()
	prompts.nextID = math.MaxUint64
	if _, err := enqueue(prompts, "session", prompt.Message{Text: "must not wrap"}); err == nil {
		t.Fatal("exhausted queue identity sequence wrapped")
	}
	if snapshot := prompts.Snapshot("session"); len(snapshot.Entries) != 0 {
		t.Fatalf("identity exhaustion mutated queue: %+v", snapshot)
	}
}

func sameDispatchReservation(left, right State) bool {
	leftID, leftPresent := left.DispatchingID()
	rightID, rightPresent := right.DispatchingID()
	return leftPresent == rightPresent && (!leftPresent || leftID == rightID)
}

func TestQueueKeepsSessionQueuesIsolatedAndSnapshotsDetached(t *testing.T) {
	prompts := New()
	first, err := enqueue(prompts, "one", prompt.Message{Text: "first"})
	if err != nil {
		t.Fatal(err)
	}
	if _, enqueueErr := enqueue(prompts, "two", prompt.Message{Text: "other session"}); enqueueErr != nil {
		t.Fatal(enqueueErr)
	}
	second, err := enqueue(prompts, "one", prompt.Message{Text: "second"})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := prompts.Snapshot("one")
	if len(snapshot.Entries) != 2 || snapshot.Entries[0].ID != first.ID || snapshot.Entries[1].ID != second.ID {
		t.Fatalf("session one queue = %+v", snapshot.Entries)
	}
	snapshot.Entries[0].Message.Text = "mutated"
	if current := prompts.Snapshot("one").Entries; len(current) != 2 || current[0].Message.Text != "first" {
		t.Fatalf("snapshot mutated queue: %+v", current)
	}
	if got := prompts.Snapshot("two").Entries; len(got) != 1 || got[0].Message.Text != "other session" {
		t.Fatalf("session two queue = %+v", got)
	}
}

func TestQueueRejectsDuplicateOrReusedCommandIdentitiesWithoutChangingState(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "first"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "second"})
	before := prompts.State("session")
	if _, err := prompts.EnqueueCommand(first.CommandID, "session", prompt.Message{Text: "another payload"}, prompt.RunOptions{}); err == nil {
		t.Fatal("duplicate command identity entered the FIFO")
	}
	if err := prompts.Update("session", first.ID, prompt.Message{Text: "edited"}, first.CommandID); err == nil {
		t.Fatal("an edit reused the original command identity")
	}
	if err := prompts.Update("session", second.ID, prompt.Message{Text: "edited"}, first.CommandID); err == nil {
		t.Fatal("an edit took another entry's command identity")
	}
	if after := prompts.State("session"); !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected command identity changed the queue: %+v", after)
	}
}

func TestQueueUpdatesMovesRemovesAndClearsByStableIdentity(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "first"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "second"})
	third, _ := enqueue(prompts, "session", prompt.Message{Text: "third"})

	if err := prompts.Update("session", second.ID, prompt.Message{Text: "edited"}, nextCommandID(prompts)); err != nil {
		t.Fatal(err)
	}
	if err := prompts.Move("session", third.ID, -2); err != nil {
		t.Fatal(err)
	}
	if err := prompts.Move("session", third.ID, -1); !errors.Is(err, ErrMoveUnavailable) {
		t.Fatalf("moving past the front returned %v", err)
	}
	removed, err := prompts.Remove("session", first.ID)
	if err != nil || removed.ID != first.ID {
		t.Fatalf("removed = %+v, %v", removed, err)
	}
	got := prompts.Snapshot("session").Entries
	if len(got) != 2 || got[0].ID != third.ID || got[1].ID != second.ID || got[1].Message.Text != "edited" {
		t.Fatalf("queue after mutations = %+v", got)
	}
	if count := prompts.Clear("session"); count != 2 {
		t.Fatalf("cleared %d entries", count)
	}
	if snapshot := prompts.Snapshot("session"); len(snapshot.Entries) != 0 {
		t.Fatalf("cleared queue still has entries: %+v", snapshot)
	}
}

func TestQueuePromotesAnEntryWithoutChangingItsIdentity(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "first"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "second"})
	third, _ := enqueue(prompts, "session", prompt.Message{Text: "third"})

	if err := prompts.Promote("session", third.ID); err != nil {
		t.Fatal(err)
	}
	got := prompts.Snapshot("session").Entries
	if len(got) != 3 || got[0].ID != third.ID || got[1].ID != first.ID || got[2].ID != second.ID {
		t.Fatalf("promoted queue = %+v", got)
	}
	before := prompts.Snapshot("session")
	if err := prompts.Promote("session", third.ID); err != nil {
		t.Fatal(err)
	}
	if got := prompts.Snapshot("session"); len(got.Entries) != len(before.Entries) || got.Entries[0].ID != before.Entries[0].ID {
		t.Fatalf("promoting the front entry changed the queue: before=%+v after=%+v", before, got)
	}
}

func TestQueueHoldsTheFrontEntryUntilEditingReleasesIt(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "first"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "second"})
	if err := prompts.Hold("session", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := prompts.BeginDispatch("session"); ok {
		t.Fatal("held front entry was dispatchable")
	}
	snapshot := prompts.Snapshot("session")
	if len(snapshot.Entries) != 2 || !snapshot.Entries[0].Held || snapshot.Entries[1].ID != second.ID {
		t.Fatalf("held snapshot = %+v", snapshot.Entries)
	}
	if err := prompts.Hold("session", first.ID); !errors.Is(err, ErrEntryHeld) {
		t.Fatalf("second hold returned %v", err)
	}
	if err := prompts.Release("session", first.ID); err != nil {
		t.Fatal(err)
	}
	if err := prompts.Release("session", first.ID); err != nil {
		t.Fatalf("idempotent release returned %v", err)
	}
	if next, ok := prompts.BeginDispatch("session"); !ok || next.ID != first.ID || next.Held {
		t.Fatalf("released next entry = %+v, %v", next, ok)
	}
	prompts.ReleaseDispatch("session")
}

func TestQueueRejectsInvalidMessagesWithoutMutation(t *testing.T) {
	prompts := New()
	if _, err := enqueue(prompts, "", prompt.Message{Text: "valid"}); !errors.Is(err, ErrSessionIDRequired) {
		t.Fatalf("empty session returned %v", err)
	}
	if _, err := enqueue(prompts, "session", prompt.Message{}); err == nil {
		t.Fatal("empty message was accepted")
	}
	if snapshot := prompts.Snapshot("session"); len(snapshot.Entries) != 0 {
		t.Fatalf("invalid enqueue mutated queue: %+v", snapshot)
	}
}

func TestQueueRejectsInvalidRunOptionsWithoutMutation(t *testing.T) {
	prompts := New()
	existing, err := enqueue(prompts, "session", prompt.Message{Text: "existing"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := prompts.BeginDispatch("session"); !ok {
		t.Fatal("queue did not reserve its existing entry")
	}
	before := prompts.State("session")
	invalid := prompt.RunOptions{Provider: "deepseek"}
	command := prompt.StartRun{
		CommandID: replay.CommandID("cli_11111111111111111111111111111111"),
		SessionID: "session", Message: prompt.Message{Text: "invalid"}, Options: invalid,
	}
	if _, err := prompts.EnqueueCommand(command.CommandID, command.SessionID, command.Message, command.Options); err == nil {
		t.Fatal("queue accepted invalid run options")
	}
	if err := prompts.Restore("session", []prompt.StartRun{command}, command.CommandID); err == nil {
		t.Fatal("durable restore accepted invalid run options")
	}
	entryID := EntryID{value: 99}
	state := State{Entries: []Entry{{
		ID: entryID, CommandID: command.CommandID, SessionID: command.SessionID,
		Message: command.Message, Options: command.Options,
	}}, Dispatching: new(entryID)}
	if err := prompts.RestoreState("session", state); err == nil {
		t.Fatal("transaction restore accepted invalid run options")
	}
	after := prompts.State("session")
	if len(after.Entries) != 1 || after.Entries[0].ID != existing.ID || !sameDispatchReservation(after, before) ||
		after.Entries[0].CommandID != before.Entries[0].CommandID {
		t.Fatalf("invalid options mutated queue: before=%+v after=%+v", before, after)
	}
}

func TestQueueRestoresAnExactSnapshotAfterARejectedTransaction(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "first"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "second"})
	if err := prompts.Hold("session", first.ID); err != nil {
		t.Fatal(err)
	}
	before := prompts.State("session")

	if err := prompts.Update("session", first.ID, prompt.Message{Text: "edited"}, nextCommandID(prompts)); err != nil {
		t.Fatal(err)
	}
	if err := prompts.Release("session", first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := prompts.Remove("session", second.ID); err != nil {
		t.Fatal(err)
	}
	if err := prompts.RestoreState("session", before); err != nil {
		t.Fatal(err)
	}

	after := prompts.Snapshot("session")
	if len(after.Entries) != 2 || after.Entries[0].ID != first.ID || after.Entries[1].ID != second.ID {
		t.Fatalf("restored queue = %+v", after.Entries)
	}
	if after.Entries[0].CommandID != first.CommandID || after.Entries[0].Message.Text != "first" || !after.Entries[0].Held {
		t.Fatalf("restored first entry = %+v", after.Entries[0])
	}
	next, err := enqueue(prompts, "session", prompt.Message{Text: "third"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID == next.ID {
		t.Fatalf("new entry reused identity %s after restoring %s", next.ID, second.ID)
	}
}

func TestQueueRestoresDurableCommandsWithFreshLocalIdentities(t *testing.T) {
	prompts := New()
	if _, err := enqueue(prompts, "other", prompt.Message{Text: "advance local identity"}); err != nil {
		t.Fatal(err)
	}
	commands := []prompt.StartRun{
		{CommandID: replay.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: prompt.Message{Text: "first"}, Options: prompt.RunOptions{}},
		{CommandID: replay.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: prompt.Message{Text: "second"}, Options: prompt.RunOptions{}},
	}
	if err := prompts.Restore("session", commands, ""); err != nil {
		t.Fatal(err)
	}
	snapshot := prompts.Snapshot("session")
	if len(snapshot.Entries) != 2 || snapshot.Entries[0].ID.Validate() != nil ||
		snapshot.Entries[0].ID == snapshot.Entries[1].ID || snapshot.Entries[0].CommandID != commands[0].CommandID ||
		snapshot.Entries[1].Message.Text != commands[1].Message.Text {
		t.Fatalf("restored durable queue = %+v", snapshot)
	}
	if err := prompts.Restore("session", nil, ""); err != nil {
		t.Fatal(err)
	}
	if got := prompts.State("session"); len(got.Entries) != 0 || got.Dispatching != nil {
		t.Fatalf("empty restore left queue state: %+v", got)
	}
}

func TestQueueRestoresADurableDispatchReservationAtomically(t *testing.T) {
	prompts := New()
	commands := []prompt.StartRun{
		{CommandID: replay.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: prompt.Message{Text: "opening"}, Options: prompt.RunOptions{}},
		{CommandID: replay.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: prompt.Message{Text: "queued"}, Options: prompt.RunOptions{}},
	}
	if err := prompts.Restore("session", commands, commands[0].CommandID); err != nil {
		t.Fatal(err)
	}
	dispatching, ok := prompts.Dispatching("session")
	if !ok || dispatching.CommandID != commands[0].CommandID {
		t.Fatalf("restored dispatch = %+v, %t", dispatching, ok)
	}
	if _, err := prompts.Remove("session", dispatching.ID); !errors.Is(err, ErrEntryDispatching) {
		t.Fatalf("restored dispatch removal returned %v", err)
	}
	if err := prompts.Promote("session", prompts.Snapshot("session").Entries[1].ID); err != nil {
		t.Fatal(err)
	}
	if first := prompts.Snapshot("session").Entries[0]; first.CommandID != commands[0].CommandID {
		t.Fatalf("priority edit crossed restored dispatch: %+v", prompts.State("session"))
	}
}

func TestQueueRejectsAnInvalidDurableDispatchWithoutMutation(t *testing.T) {
	prompts := New()
	existing, _ := enqueue(prompts, "session", prompt.Message{Text: "existing"})
	before := prompts.State("session")
	commands := []prompt.StartRun{
		{CommandID: replay.CommandID("cli_11111111111111111111111111111111"), SessionID: "session", Message: prompt.Message{Text: "first"}, Options: prompt.RunOptions{}},
		{CommandID: replay.CommandID("cli_22222222222222222222222222222222"), SessionID: "session", Message: prompt.Message{Text: "second"}, Options: prompt.RunOptions{}},
	}
	if err := prompts.Restore("session", commands, commands[1].CommandID); err == nil {
		t.Fatal("queue accepted a non-front durable dispatch")
	}
	after := prompts.State("session")
	if len(after.Entries) != 1 || after.Entries[0].ID != existing.ID || after.Entries[0].CommandID != before.Entries[0].CommandID ||
		!sameDispatchReservation(after, before) {
		t.Fatalf("invalid durable restore mutated queue: before=%+v after=%+v", before, after)
	}
}

func TestDispatchReservationProtectsRuntimeIdentityFromPriorityEdits(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "opening"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "send next"})
	third, _ := enqueue(prompts, "session", prompt.Message{Text: "leave last"})

	dispatching, ok := prompts.BeginDispatch("session")
	if !ok || dispatching.ID != first.ID {
		t.Fatalf("dispatch reservation = %+v, %t", dispatching, ok)
	}
	if err := prompts.Promote("session", third.ID); err != nil {
		t.Fatal(err)
	}
	entries := prompts.Snapshot("session").Entries
	state := prompts.State("session")
	dispatchingID, reserved := state.DispatchingID()
	if len(entries) != 3 || entries[0].ID != first.ID || !reserved || dispatchingID != first.ID ||
		entries[1].ID != third.ID || entries[2].ID != second.ID {
		t.Fatalf("priority edit crossed dispatch boundary: %+v", entries)
	}
	if _, ok := prompts.BeginDispatch("session"); ok {
		t.Fatal("queue exposed a second dispatch while the first was reserved")
	}
	if _, err := prompts.Remove("session", first.ID); !errors.Is(err, ErrEntryDispatching) {
		t.Fatalf("dispatching removal returned %v", err)
	}
	if err := prompts.Move("session", third.ID, -1); !errors.Is(err, ErrMoveUnavailable) {
		t.Fatalf("move across dispatch boundary returned %v", err)
	}

	removed, err := prompts.RetireCommand("session", first.CommandID)
	if err != nil || removed.ID != first.ID {
		t.Fatalf("committed dispatch = %+v, %v", removed, err)
	}
	if next, ok := prompts.BeginDispatch("session"); !ok || next.ID != third.ID {
		t.Fatalf("next after dispatch = %+v, %t", next, ok)
	}
}

func TestRestoreStatePreservesDispatchReservation(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "opening"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "queued"})
	if _, ok := prompts.BeginDispatch("session"); !ok {
		t.Fatal("could not reserve the front entry")
	}
	before := prompts.State("session")

	if _, err := prompts.RetireCommand("session", first.CommandID); err != nil {
		t.Fatal(err)
	}
	if err := prompts.RestoreState("session", before); err != nil {
		t.Fatal(err)
	}
	dispatching, ok := prompts.Dispatching("session")
	if !ok || dispatching.ID != first.ID || dispatching.CommandID != first.CommandID {
		t.Fatalf("restored dispatch = %+v, %t", dispatching, ok)
	}
	entries := prompts.Snapshot("session").Entries
	state := prompts.State("session")
	dispatchingID, reserved := state.DispatchingID()
	if len(entries) != 2 || entries[1].ID != second.ID || !reserved || dispatchingID != first.ID {
		t.Fatalf("restored snapshot = %+v", entries)
	}
}

func TestRestoreStateRejectsInvalidDispatchWithoutChangingTheQueue(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "first"})
	second, _ := enqueue(prompts, "session", prompt.Message{Text: "second"})
	before := prompts.State("session")
	invalid := before
	invalid.Dispatching = new(second.ID)

	if err := prompts.RestoreState("session", invalid); err == nil {
		t.Fatal("snapshot reserved a non-front entry")
	}
	after := prompts.State("session")
	if !sameDispatchReservation(after, before) || len(after.Entries) != 2 ||
		after.Entries[0].ID != first.ID || after.Entries[1].ID != second.ID {
		t.Fatalf("invalid restore mutated queue: before=%+v after=%+v", before, after)
	}
}

func TestRejectedDispatchIsReidentifiedAndReleasedAtomically(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "retry me"})
	if _, ok := prompts.BeginDispatch("session"); !ok {
		t.Fatal("could not reserve dispatch")
	}
	replacement := replay.CommandID("cli_99999999999999999999999999999999")
	if err := prompts.RequeueDispatch("session", first.CommandID, replacement); err != nil {
		t.Fatal(err)
	}
	if _, reserved := prompts.State("session").DispatchingID(); reserved {
		t.Fatal("requeued dispatch retained its reservation")
	}
	next, ok := prompts.BeginDispatch("session")
	if !ok || next.ID != first.ID || next.CommandID != replacement {
		t.Fatalf("requeued dispatch = %+v, %t", next, ok)
	}
}

func TestReleasingDispatchReturnsTheSameCommandToFIFO(t *testing.T) {
	prompts := New()
	first, _ := enqueue(prompts, "session", prompt.Message{Text: "opening"})
	if prompts.ReleaseDispatch("session") {
		t.Fatal("empty dispatch release reported a state change")
	}
	if _, ok := prompts.BeginDispatch("session"); !ok {
		t.Fatal("could not reserve dispatch")
	}
	if !prompts.ReleaseDispatch("session") {
		t.Fatal("dispatch release reported no state change")
	}
	if next, ok := prompts.BeginDispatch("session"); !ok || next.ID != first.ID || next.CommandID != first.CommandID {
		t.Fatalf("released FIFO entry = %+v, %t", next, ok)
	}
}

func nextCommandID(q *Queue) replay.CommandID {
	var entropy [replay.CommandIDEntropyBytes]byte
	binary.BigEndian.PutUint64(entropy[:8], q.nextID+1)
	return replay.NewCommandID(entropy)
}
