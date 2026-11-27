package main

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestAttachmentSnapshotFollowsPendingRun(t *testing.T) {
	directory := t.TempDir()
	open := func() *workbench.Store {
		t.Helper()
		store, err := workbenchFactory(directory)("")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
		return store
	}
	store := open()
	database := inspectWorkbenchDatabase(t, directory)
	assertSnapshots := func(want int) {
		t.Helper()
		var count int
		if err := database.QueryRow(`SELECT count(*) FROM authoring_records WHERE key GLOB 'input/*'`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("durable input snapshots = %d, want %d", count, want)
		}
	}
	const session = "ses_attachment"
	const otherSession = "ses_other_attachment"
	const attachmentContent = "original attached text"
	const attachmentName = "attachment.txt"
	message := authoring.Message{Text: "read the attachment", Attachments: []authoring.Attachment{{
		ID: attachmentName, Name: attachmentName, Kind: protocol.ContentBlockText,
		Path: filepath.Join(directory, attachmentName), Size: int64(len(attachmentContent)),
	}}}
	blocks := []protocol.ContentBlock{
		{Type: protocol.ContentBlockText, Text: message.Text},
		{Type: protocol.ContentBlockText, Text: attachmentContent},
	}
	prepared, err := store.PrepareInput(t.Context(), message, blocks)
	if err != nil {
		t.Fatal(err)
	}
	assertSnapshots(0)
	queue := store.Queue()
	stage := func(sessionID string) authoring.CommandID {
		t.Helper()
		commandID := mutation.NewCommandID()
		if _, err := queue.EnqueueCommand(commandID, sessionID, message, authoring.RunOptions{}); err != nil {
			t.Fatal(err)
		}
		if _, claimed := queue.BeginDispatch(sessionID); !claimed {
			t.Fatal("did not claim the queued command")
		}
		if _, err := queue.PrepareDispatch(sessionID, commandID, authoring.UnprotectedGuard(), prepared); err != nil {
			t.Fatal(err)
		}
		return commandID
	}
	commandID := stage(session)
	otherCommandID := stage(otherSession)
	assertSnapshots(2)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = open()
	queue = store.Queue()
	assertRecovered := func(sessionID string) {
		t.Helper()
		pending, found := queue.Head(sessionID)
		if !found {
			t.Fatalf("pending command in %s disappeared after reopen", sessionID)
		}
		replayed, err := pending.ReplayCommand()
		if err != nil || !slices.Equal(replayed.Input, blocks) {
			t.Fatalf("recovered input = %+v, %v", replayed.Input, err)
		}
	}
	assertRecovered(session)
	if retired, err := queue.SettleDispatch(session, commandID); err != nil || !retired {
		t.Fatalf("settlement = %t, %v", retired, err)
	}
	assertSnapshots(1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = open()
	queue = store.Queue()
	assertRecovered(otherSession)
	if retired, err := queue.SettleDispatch(otherSession, otherCommandID); err != nil || !retired {
		t.Fatalf("settlement of other command = %t, %v", retired, err)
	}
	assertSnapshots(0)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if history := open().History(); len(history) != 1 || !history[0].Equal(message) {
		t.Fatalf("reopened history = %+v, want one recall entry for the repeated prompt", history)
	}
}
