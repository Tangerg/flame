package terminal

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestDraftClearCommitsHistoryAndRetirementTogether(t *testing.T) {
	connection, source := openSessionNavigationRuntime(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	directory := t.TempDir()
	store, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	const draftText = "keep this draft until clearing succeeds"
	if err := store.SaveDraft(source.ID, authoring.Message{Text: draftText}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	config := sessionNavigationConfig(connection, connection, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, currentStore := runUIFromConfig(t, config)
	defer stop()
	host.Shows(t, draftText)
	database := inspectSessionWorkbench(t, directory)
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER refuse_draft_clear
		BEFORE DELETE ON authoring_records WHEN old.key GLOB 'session/*'
		BEGIN SELECT RAISE(ABORT, 'draft clear refused'); END`); err != nil {
		t.Fatal(err)
	}
	clearDraft := func() {
		t.Helper()
		if !host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}) {
			t.Fatal("terminal refused to clear the draft")
		}
	}
	clearDraft()
	host.Shows(t, "workbench: commit workbench state")
	host.Shows(t, draftText)
	if history := currentStore().History(); len(history) != 0 {
		t.Fatalf("failed clear recorded a history entry: %+v", history)
	}
	if _, err := database.ExecContext(ctx, "DROP TRIGGER refuse_draft_clear"); err != nil {
		t.Fatal(err)
	}
	clearDraft()
	host.Hides(t, draftText)
	stop()
	reopened, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := reopened.Draft(source.ID); exists {
		t.Fatal("the cleared draft survives reopening")
	}
	if history := reopened.History(); len(history) != 1 || history[0].Text != draftText {
		t.Fatalf("reopened history = %+v, want one entry for the cleared draft", history)
	}
}

func TestStashCommitsVisibleDraftWithoutSavingItFirst(t *testing.T) {
	connection, source := openSessionNavigationRuntime(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	directory := t.TempDir()
	store, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	const stored = "draft saved earlier"
	const edit = " with a newer visible edit"
	if err := store.SaveDraft(source.ID, authoring.Message{Text: stored}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	config := sessionNavigationConfig(connection, connection, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, _ := runUIFromConfig(t, config)
	defer stop()
	host.Shows(t, stored)
	database := inspectSessionWorkbench(t, directory)
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER refuse_intermediate_draft_write
		BEFORE UPDATE ON authoring_records WHEN new.key GLOB 'session/*'
		BEGIN SELECT RAISE(ABORT, 'intermediate draft write refused'); END`); err != nil {
		t.Fatal(err)
	}
	if !host.Press(input.End) || !host.Type(edit) {
		t.Fatal("terminal refused the new draft text")
	}
	host.Shows(t, stored+edit)
	if !host.Send(input.Key{Code: input.Character, Rune: 'p', Mods: input.Ctrl}) {
		t.Fatal("terminal refused to open the command palette")
	}
	host.Shows(t, "search commands")
	if !host.Type("stash the current prompt") {
		t.Fatal("terminal refused to search for stash")
	}
	host.Shows(t, "❯ /stash")
	if !host.Press(input.Enter) {
		t.Fatal("terminal refused the stash command")
	}
	host.Shows(t, "stashed prompt")
	stop()
	reopened, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := reopened.Draft(source.ID); exists {
		t.Fatal("the stashed draft survives reopening")
	}
	if stashes := reopened.Stashes(); len(stashes) != 1 || stashes[0].Message.Text != stored+edit {
		t.Fatalf("reopened stashes = %+v, want the visible edited draft", stashes)
	}
}
