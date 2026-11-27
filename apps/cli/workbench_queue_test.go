package main

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestWorkbenchQueueReopensItsCommittedEntries(t *testing.T) {
	const sessionID = "ses_01"
	open := workbenchFactory(t.TempDir())
	openQueue := func() (*workbench.Store, *workbench.Queue) {
		t.Helper()
		store, err := open("")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
		return store, store.Queue()
	}
	store, queue := openQueue()
	first, err := queue.EnqueueCommand(mutation.NewCommandID(), sessionID, authoring.Message{Text: "first"}, authoring.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := queue.EnqueueCommand(mutation.NewCommandID(), sessionID, authoring.Message{Text: "second"}, authoring.RunOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := queue.Promote(sessionID, second.ID); err != nil {
		t.Fatal(err)
	}
	if err := queue.Hold(sessionID, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopenedStore, reopened := openQueue()
	entries := reopened.Snapshot(sessionID).Entries
	if len(entries) != 2 || entries[0].ID != second.ID || entries[1].ID != first.ID {
		t.Fatalf("reopened entries = %+v, want %s then %s", entries, second.ID, first.ID)
	}
	if entries[1].Held {
		t.Fatal("the previous terminal's editing claim survived reopening")
	}
	head, started := reopened.BeginDispatch(sessionID)
	if !started || head.Command.CommandID != second.CommandID || head.Command.Message.Text != second.Message.Text {
		t.Fatalf("reopened queue offered %+v, %t, want %+v", head, started, second)
	}

	prepared, err := reopenedStore.PrepareInput(t.Context(), head.Command.Message, []protocol.ContentBlock{
		{Type: protocol.ContentBlockText, Text: head.Command.Message.Text},
	})
	if err != nil {
		t.Fatal(err)
	}
	replay, err := authoring.NewProtectedGuard("original-store", time.Unix(4_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.PrepareDispatch(sessionID, head.Command.CommandID, replay, prepared); err != nil {
		t.Fatal(err)
	}
	canceled, err := reopened.StageCancellation(sessionID, head.Command.CommandID, replay)
	if err != nil {
		t.Fatal(err)
	}
	if err := reopenedStore.Close(); err != nil {
		t.Fatal(err)
	}

	recoveredStore, recovered := openQueue()
	recovered.ReleasePreparation(sessionID)
	pending, found := recovered.Head(sessionID)
	if !found || pending.Dispatch == nil || pending.Dispatch.Replay != replay ||
		pending.Cancellation() == nil || *pending.Cancellation() != *canceled.Cancellation() {
		t.Fatalf("recovered dispatch = %+v, want original cancellation %v", pending, canceled.Cancellation())
	}
	original, err := pending.ReplayCommand()
	if err != nil || !original.Equal(canceled.Command) {
		t.Fatalf("replay = %+v, %v, want %+v", original, err, canceled.Command)
	}
	if _, started := recovered.BeginDispatch(sessionID); started {
		t.Fatal("another prompt bypassed the unsettled cancellation")
	}
	if _, _, err := recovered.SettleDispatch(sessionID, second.CommandID); !errors.Is(err, workbench.ErrDispatchCanceling) {
		t.Fatalf("start acknowledgement settled a cancellation: %v", err)
	}
	if _, retired, err := recovered.Retire(sessionID, second.CommandID); err != nil || !retired {
		t.Fatalf("cancellation retirement = %t, %v", retired, err)
	}
	edited := authoring.Message{Text: "edited first prompt"}
	if err := recovered.Update(sessionID, first.ID, edited); err != nil {
		t.Fatal(err)
	}
	waiting := recovered.Snapshot(sessionID).Entries
	if len(waiting) != 1 || waiting[0].ID != first.ID ||
		waiting[0].CommandID == first.CommandID || !waiting[0].Message.Equal(edited) {
		t.Fatalf("edited queue = %+v", waiting)
	}
	if err := recoveredStore.Close(); err != nil {
		t.Fatal(err)
	}
	settledStore, settled := openQueue()
	history := settledStore.History()
	if len(history) != 1 || !history[0].Equal(second.Message) {
		t.Fatalf("reopened history = %+v, want the retired prompt", history)
	}
	if entries := settled.Snapshot(sessionID).Entries; len(entries) != 1 || !entries[0].Message.Equal(edited) {
		t.Fatalf("reopened queue = %+v, want only the remaining prompt", entries)
	}
}
