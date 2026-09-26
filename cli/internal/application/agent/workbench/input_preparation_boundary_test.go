package workbench

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func TestPreparedInputPublicationKeepsItsPayloadAfterStateRootReplacement(t *testing.T) {
	parent := t.TempDir()
	directory := filepath.Join(parent, "state")
	store, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	pending := steerTestPending(t, "ses_preparing", steerTestAttachment(t.TempDir()))
	source := agent.Message{Text: "/steer " + pending.Message().Text, Attachments: pending.Message().Attachments}
	if err := store.SaveDraft(pending.SessionID(), source); err != nil {
		t.Fatal(err)
	}
	prepared := preparedTestInput(t, store, pending.Message(), pending.Command().Input)

	// An external root replacement cannot move this owner's journal. A delayed
	// preparation must not publish a record whose immutable input stayed behind.
	if err := os.Rename(directory, filepath.Join(parent, "old-state")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	stageErr := store.StagePendingSteer(pending, source, prepared)
	reopened, err := OpenDirectory(directory, Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	replayed, found := reopened.PendingSteer(pending.SessionID())
	if stageErr != nil {
		if found {
			t.Fatalf("rejected preparation still published a command: %v", stageErr)
		}
		if draft, found := store.Draft(pending.SessionID()); !found || !draft.Equal(source) {
			t.Fatalf("rejected preparation lost its editable source: %+v, found=%t", draft, found)
		}
		return
	}
	command, err := replayed.ReplayCommand()
	if !found || err != nil || !command.Equal(pending.Command()) {
		t.Fatalf("published command lost its prepared input after root replacement: found=%t, error=%v", found, err)
	}
}
