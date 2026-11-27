package terminal

import (
	"context"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestFailedSessionActivationKeepsDraftsAndTheOriginalRollbackReceipt(t *testing.T) {
	connection, source := openSessionNavigationRuntime(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	destination, err := connection.CreateSession(ctx, conversation.CreateSession{Workspace: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	const title = "Activation target"
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
	const baseline = "source draft"
	const edit = " with later changes"
	const destinationDraft = "destination draft"
	if err := store.SaveDraft(source.ID, authoring.Message{Text: baseline}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDraft(destination.ID, authoring.Message{Text: destinationDraft}); err != nil {
		t.Fatal(err)
	}
	rollback := confirmSessionActivationRollback(t, ctx, connection, store, destination.ID)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	captured := make(chan context.Context, 1)
	release := make(chan struct{})
	backend := &heldSessionActivation{
		Connection: connection, destinationID: destination.ID,
		read: heldSessionRead{captured: captured, release: release},
	}
	config := sessionNavigationConfig(connection, backend, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, currentStore := runUIFromConfig(t, config)
	defer stop()
	host.Shows(t, baseline)
	database := inspectSessionWorkbench(t, directory)
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER refuse_rollback_consumption
		BEFORE UPDATE ON authoring_records
		WHEN instr(CAST(old.body AS TEXT), '"pendingRollback"') > 0
		AND instr(CAST(new.body AS TEXT), '"pendingRollback"') = 0
		BEGIN SELECT RAISE(ABORT, 'rollback receipt consumption refused'); END`); err != nil {
		t.Fatal(err)
	}
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
	awaitSessionRead(t, ctx, captured, "destination snapshot")
	if !host.Press(input.End) || !host.Type(edit) {
		t.Fatal("terminal refused edits during navigation")
	}
	host.Shows(t, baseline+edit)
	close(release)
	host.Shows(t, "loading session failed")
	assertUnchanged := func(store *workbench.Store) {
		t.Helper()
		if draft, _ := store.Draft(source.ID); draft.Text != baseline+edit {
			t.Fatalf("failed activation changed the source draft to %q", draft.Text)
		}
		if draft, _ := store.Draft(destination.ID); draft.Text != destinationDraft {
			t.Fatalf("failed activation changed the destination draft to %q", draft.Text)
		}
		if pending, ok := store.PendingSessionRollback(destination.ID); !ok || pending.CommandID != rollback || pending.Restoration == nil {
			t.Fatalf("failed activation changed the confirmed rollback: %+v", pending)
		}
		if workspaces := store.Workspaces(); len(workspaces) != 1 || workspaces[0].Path != source.Workspace.Path {
			t.Fatalf("failed activation changed recent workspaces: %+v", workspaces)
		}
	}
	assertUnchanged(currentStore())
	stop()
	reopened, err := openTestWorkbench(t, directory)
	if err != nil {
		t.Fatal(err)
	}
	assertUnchanged(reopened)
}

func confirmSessionActivationRollback(t *testing.T, ctx context.Context, connection *runtimebinding.Connection, store *workbench.Store, destinationID string) authoring.CommandID {
	t.Helper()
	snapshot, err := connection.GetSession(ctx, destinationID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := session.PreviewRollback(snapshot, conversation.RollbackTarget{SessionID: destinationID, Scope: protocol.RestoreHistory})
	if err != nil {
		t.Fatal(err)
	}
	policy, err := mutation.NewReplayPolicy(connection.Profile().IdempotencyLimits(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := session.Rollback(ctx, connection, store, preview, policy, mutation.AcknowledgementBackoff())
	if err != nil || result.Outcome != mutation.Confirmed {
		t.Fatalf("Runtime rollback = %+v, %v", result, err)
	}
	pending, ok := store.PendingSessionRollback(destinationID)
	if !ok || pending.Restoration == nil {
		t.Fatal("Workbench did not retain the confirmed rollback")
	}
	return pending.CommandID
}

type heldSessionActivation struct {
	*runtimebinding.Connection
	destinationID string
	read          heldSessionRead
}

func (r *heldSessionActivation) GetSession(ctx context.Context, id string) (conversation.SessionSnapshot, error) {
	snapshot, err := r.Connection.GetSession(ctx, id)
	if err == nil && id == r.destinationID {
		return r.read.wait(ctx, snapshot)
	}
	return snapshot, err
}
