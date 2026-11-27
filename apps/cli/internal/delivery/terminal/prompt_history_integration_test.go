package terminal

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestPromptHistoryDoesNotRestoreAnotherSessionsDraft(t *testing.T) {
	connection, source := openSessionNavigationRuntime(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	destination, err := connection.CreateSession(ctx, conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	const title = "History navigation destination"
	if _, err := connection.UpdateSession(ctx, conversation.UpdateSession{
		SessionID: destination.ID, ExpectedRevision: destination.Revision, Title: new(title),
	}); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	store, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	const sourceText = "draft belonging to the source session"
	const destinationText = "draft belonging to the destination session"
	const historyText = "a previously accepted prompt"
	if err := store.MoveDraftToHistory(source.ID, authoring.Message{Text: historyText}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft(source.ID, authoring.Message{Text: sourceText}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft(destination.ID, authoring.Message{Text: destinationText}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	config := sessionNavigationConfig(connection, connection, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, _ := runUIFromConfig(t, config)
	defer stop()
	host.Shows(t, sourceText)
	browse := func(direction input.Code, expected string) {
		t.Helper()
		if !host.Send(input.Key{Code: direction, Mods: input.Alt}) {
			t.Fatal("terminal refused to browse prompt history")
		}
		host.Shows(t, expected)
	}
	browse(input.Up, historyText)
	browse(input.Down, sourceText)
	browse(input.Up, historyText)
	if !host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl}) {
		t.Fatal("terminal refused to open Sessions")
	}
	host.Shows(t, "Sessions · Center")
	if !host.Type(title) {
		t.Fatal("terminal refused to search Sessions")
	}
	host.Shows(t, destination.ID)
	if !host.Press(input.Enter) {
		t.Fatal("terminal refused the destination")
	}
	host.Shows(t, destinationText)
	const edit = " and a later edit"
	if !host.Send(input.Key{Code: input.Down, Mods: input.Alt}) || !host.Press(input.End) || !host.Type(edit) {
		t.Fatal("terminal refused to continue editing the destination draft")
	}
	host.Shows(t, destinationText+edit)
	stop()
	reopened, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	if draft, _ := reopened.Draft(destination.ID); draft.Text != destinationText+edit {
		t.Fatalf("destination's durable draft = %q, want %q", draft.Text, destinationText+edit)
	}
}
