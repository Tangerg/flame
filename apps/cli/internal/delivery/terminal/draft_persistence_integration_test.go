package terminal

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestAutosaveRetriesSameDraftAfterWriteFailure(t *testing.T) {
	connection, source := openSessionNavigationRuntime(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	directory := t.TempDir()
	store, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	const baseline = "saved draft"
	if err := store.SaveDraft(source.ID, authoring.Message{Text: baseline}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	config := sessionNavigationConfig(connection, connection, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, currentStore := runUIFromConfig(t, config)
	defer stop()
	host.Shows(t, baseline)
	database := inspectSessionWorkbench(t, directory)
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER refuse_draft_write
		BEFORE UPDATE ON authoring_records
		BEGIN SELECT RAISE(ABORT, 'draft write refused'); END`); err != nil {
		t.Fatal(err)
	}
	choosePreview := func() {
		t.Helper()
		if !host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl}) {
			t.Fatal("terminal refused to open commands")
		}
		host.Shows(t, "search commands")
		if !host.Type("preview") {
			t.Fatal("terminal refused to search commands")
		}
		host.Shows(t, "preview a file")
		if !host.Press(input.Enter) {
			t.Fatal("terminal refused the preview command")
		}
		host.Hides(t, "Commands")
		host.Shows(t, "/preview")
	}
	choosePreview()
	const writeFailure = "workbench: commit workbench state"
	host.Shows(t, writeFailure)
	if draft, _ := currentStore().Draft(source.ID); draft.Text != baseline {
		t.Fatalf("failed autosave changed the stored draft to %q", draft.Text)
	}
	if _, err := database.ExecContext(ctx, "DROP TRIGGER refuse_draft_write"); err != nil {
		t.Fatal(err)
	}
	choosePreview()
	host.Until(t, "the same draft to be saved after the write failure", func() bool {
		draft, _ := currentStore().Draft(source.ID)
		return draft.Text == "/preview "
	})
	host.Hides(t, writeFailure)
}
