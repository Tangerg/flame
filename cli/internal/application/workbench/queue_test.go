package workbench

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/statefile"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/prompt"
	"github.com/Tangerg/flame/cli/internal/domain/authoring/replay"
)

type queueWriteFailure struct {
	Persistence
	fail string
}

func (p *queueWriteFailure) Replace(name string, body []byte) error {
	if name == p.fail {
		return errors.New("authoring disk unavailable")
	}
	return p.Persistence.Replace(name, body)
}

func queueWithStorage(t *testing.T) (*Queue, *Store, *queueWriteFailure, string) {
	t.Helper()
	directory := t.TempDir()
	storage, err := statefile.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	writes := &queueWriteFailure{Persistence: storage}
	store, err := Open(writes, Config{})
	if err != nil {
		t.Fatal(err)
	}
	prompts, err := NewQueue(store)
	if err != nil {
		t.Fatal(err)
	}
	for index, message := range []string{"opening", "second", "promote me"} {
		var entropy [replay.CommandIDEntropyBytes]byte
		entropy[0] = byte(index + 1)
		if _, err := prompts.EnqueueCommand(replay.NewCommandID(entropy), "session", prompt.Message{Text: message}, prompt.RunOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	return prompts, store, writes, directory
}

func TestQueueMutationRollbackPreservesTheDispatchReservation(t *testing.T) {
	prompts, store, writes, directory := queueWithStorage(t)
	first, ok := prompts.BeginDispatch("session")
	if !ok {
		t.Fatal("first command was not reserved")
	}
	if err := store.MarkPendingRunDispatching("session", first.CommandID, replay.UnprotectedGuard(), nil); err != nil {
		t.Fatal(err)
	}
	before := prompts.State("session")
	pending := store.PendingRuns("session")
	writes.fail = store.sessionStateName("session")
	if err := prompts.Promote("session", before.Entries[2].ID); err == nil {
		t.Fatal("queue priority changed without a durable replacement")
	}
	if got := prompts.State("session"); !reflect.DeepEqual(got, before) {
		t.Fatalf("failed edit changed the live FIFO or reservation: %+v", got)
	}
	writes.fail = ""
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if got := reopened.PendingRuns("session"); !reflect.DeepEqual(got, pending) {
		t.Fatalf("failed edit changed the durable FIFO: %+v", got)
	}
}

func TestDurableQueueKeepsTheOpeningCommandAheadOfPriorityEdits(t *testing.T) {
	prompts, store, _, _ := queueWithStorage(t)
	first, _ := prompts.BeginDispatch("session")
	if err := store.MarkPendingRunDispatching("session", first.CommandID, replay.UnprotectedGuard(), nil); err != nil {
		t.Fatal(err)
	}
	last := prompts.Snapshot("session").Entries[2]
	if err := prompts.Promote("session", last.ID); err != nil {
		t.Fatal(err)
	}
	pending := store.PendingRuns("session")
	if len(pending) != 3 || pending[0].Command.CommandID != first.CommandID || pending[0].State != PendingRunDispatching || pending[1].Command.CommandID != last.CommandID {
		t.Fatalf("priority edit crossed the opening boundary: %+v", pending)
	}
	if _, retired, err := prompts.SettleDispatch("session"); err != nil || !retired {
		t.Fatalf("settlement = %t, %v", retired, err)
	}
	if next, ok := prompts.BeginDispatch("session"); !ok || next.CommandID != last.CommandID {
		t.Fatalf("promoted successor = %+v, %t", next, ok)
	}
}

func TestQueueSettlementRetainsReservationUntilBothDurableWritesSucceed(t *testing.T) {
	prompts, store, writes, _ := queueWithStorage(t)
	first, _ := prompts.BeginDispatch("session")
	if err := store.MarkPendingRunDispatching("session", first.CommandID, replay.UnprotectedGuard(), nil); err != nil {
		t.Fatal(err)
	}
	writes.fail = store.sessionStateName("session")
	if _, retired, err := prompts.SettleDispatch("session"); err == nil || retired {
		t.Fatalf("failed outbox retirement settled FIFO: %t, %v", retired, err)
	}
	if len(store.History()) != 1 {
		t.Fatal("history half of settlement did not commit")
	}
	if _, ok := prompts.BeginDispatch("session"); ok {
		t.Fatal("successor dispatched before durable settlement")
	}
	if reserved, ok := prompts.Dispatching("session"); !ok || reserved.CommandID != first.CommandID {
		t.Fatal("failed settlement lost the opening reservation")
	}
	writes.fail = ""
	if _, retired, err := prompts.SettleDispatch("session"); err != nil || !retired {
		t.Fatalf("retry settlement = %t, %v", retired, err)
	}
	if len(store.History()) != 1 || len(store.PendingRuns("session")) != 2 {
		t.Fatal("settlement retry duplicated history or lost a successor")
	}
}

func TestQueueRecoveryPreservesHeldSuccessors(t *testing.T) {
	for _, outcome := range []string{"accepted", "rejected"} {
		t.Run(outcome, func(t *testing.T) {
			prompts, store, _, directory := queueWithStorage(t)
			first, _ := prompts.BeginDispatch("session")
			if err := store.MarkPendingRunDispatching("session", first.CommandID, replay.UnprotectedGuard(), nil); err != nil {
				t.Fatal(err)
			}
			successor := prompts.Snapshot("session").Entries[1]
			if err := prompts.Hold("session", successor.ID); err != nil {
				t.Fatal(err)
			}
			before := prompts.Snapshot("session").Entries[1:]
			if outcome == "accepted" {
				if _, retired, err := prompts.Retire("session", first.CommandID); err != nil || !retired {
					t.Fatalf("accepted recovery = %t, %v", retired, err)
				}
			} else if err := prompts.RequeueDispatch("session", first.CommandID); err != nil {
				t.Fatal(err)
			}
			if _, reserved := prompts.Dispatching("session"); reserved {
				t.Fatal("reconciled command retained its dispatch reservation")
			}
			entries := prompts.Snapshot("session").Entries
			after := entries
			if outcome == "rejected" {
				if entries[0].ID != first.ID || entries[0].CommandID == first.CommandID {
					t.Fatalf("requeued command = %+v", entries[0])
				}
				after = entries[1:]
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("recovery changed successor identity, held state, or content: got %+v, want %+v", after, before)
			}
			reopened, err := OpenDirectory(directory, Config{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = reopened.Close() })
			pending := reopened.PendingRuns("session")
			if len(pending) != len(entries) {
				t.Fatalf("recovered durable queue = %+v, live queue = %+v", pending, entries)
			}
			for index, entry := range entries {
				if pending[index].State != PendingRunQueued || pending[index].Command.CommandID != entry.CommandID ||
					!reflect.DeepEqual(pending[index].Command.Message, entry.Message) {
					t.Fatalf("durable queue entry %d = %+v, live = %+v", index, pending[index], entry)
				}
			}
		})
	}
}

func TestRestoredPendingRunStateControlsQueueOwnership(t *testing.T) {
	for _, state := range []PendingRunState{PendingRunQueued, PendingRunDispatching, PendingRunCanceling} {
		t.Run(string(state), func(t *testing.T) {
			prompts, store, _, _ := queueWithStorage(t)
			first := prompts.Snapshot("session").Entries[0]
			if state != PendingRunQueued {
				if err := store.MarkPendingRunDispatching("session", first.CommandID, replay.UnprotectedGuard(), nil); err != nil {
					t.Fatal(err)
				}
			}
			if state == PendingRunCanceling {
				if _, err := store.MarkPendingRunCanceling("session", first.CommandID, replay.UnprotectedGuard()); err != nil {
					t.Fatal(err)
				}
			}
			recovered, err := NewQueue(store)
			if err != nil {
				t.Fatal(err)
			}
			if err := recovered.Restore("session"); err != nil {
				t.Fatal(err)
			}
			reserved, found := recovered.Dispatching("session")
			if found != (state != PendingRunQueued) || found && reserved.CommandID != first.CommandID {
				t.Fatalf("restored reservation = %+v, %t", reserved, found)
			}
			if state == PendingRunCanceling {
				if _, _, err := recovered.SettleDispatch("session"); !errors.Is(err, ErrDispatchCanceling) {
					t.Fatalf("cancellation was settled as an ordinary acknowledgement: %v", err)
				}
			}
		})
	}
}
