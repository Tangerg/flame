package terminal

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTerminalKeepsTheOriginalReplacementWhenLocalActivationFails(t *testing.T) {
	connection, source := openSessionNavigationRuntime(t)
	created := make(chan conversation.Session, 2)
	backend := &observedSessionCreations{Connection: connection, created: created}
	directory := t.TempDir()
	config := sessionNavigationConfig(connection, backend, source)
	config.OpenWorkbench = persistentTestWorkbench(t, directory)
	host, stop, currentStore := runUIFromConfig(t, config)
	host.Shows(t, "Ask flame")
	const draftText = "draft kept after the current session was deleted"
	if !host.Type(draftText) {
		t.Fatal("terminal refused the draft")
	}
	host.Shows(t, draftText)

	address := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(directory, "authoring.db")), RawQuery: "mode=rw"}
	database, err := sql.Open("sqlite", address.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := database.ExecContext(ctx, `CREATE TRIGGER refuse_workspace_write
		BEFORE UPDATE ON authoring_records WHEN new.key = 'workspaces'
		BEGIN SELECT RAISE(ABORT, 'workspace write refused'); END`); err != nil {
		t.Fatal(err)
	}
	if err := connection.DeleteSession(ctx, conversation.DeleteSession{SessionID: source.ID, CommandID: mutation.NewCommandID()}); err != nil {
		t.Fatal(err)
	}
	var replacement conversation.Session
	select {
	case replacement = <-created:
	case <-ctx.Done():
		t.Fatal("terminal did not create a replacement Session", context.Cause(ctx))
	}
	notice := "created replacement session · " + displayTitle(replacement) + " · could not open"
	host.Shows(t, notice)
	select {
	case duplicate := <-created:
		t.Fatalf("local activation created a second replacement Session %s after %s was committed", duplicate.ID, replacement.ID)
	default:
	}
	host.Shows(t, draftText)
	if !host.Press(input.Enter) {
		t.Fatal("terminal refused to try sending the draft")
	}
	host.Shows(t, "open a session before sending a prompt")
	host.Shows(t, draftText)
	if _, err := database.ExecContext(ctx, "DROP TRIGGER refuse_workspace_write"); err != nil {
		t.Fatal(err)
	}
	if !host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl}) {
		t.Fatal("terminal refused to open Sessions")
	}
	host.Shows(t, replacement.ID)
	if !host.Press(input.Enter) {
		t.Fatal("terminal refused to open the committed replacement")
	}
	host.Shows(t, "session · "+displayTitle(replacement))
	host.Shows(t, draftText)
	if _, err := connection.GetSession(ctx, source.ID); !errors.Is(err, conversation.ErrSessionNotFound) {
		t.Fatalf("deleted source read returned %v", err)
	}
	page, err := connection.ListSessions(ctx, conversation.SessionQuery{PageSize: conversation.DefaultPageSize()})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != replacement.ID {
		t.Fatalf("Runtime Sessions = %+v, error = %v, want only %s", page.Items, err, replacement.ID)
	}
	store := currentStore()
	if draft, _ := store.Draft(replacement.ID); draft.Text != draftText {
		t.Fatalf("replacement draft = %q, want %q", draft.Text, draftText)
	}
	if _, exists := store.Draft(source.ID); exists {
		t.Fatal("the deleted Session still owns the transferred draft")
	}
	stop()
}

type observedSessionCreations struct {
	*runtimebinding.Connection
	created chan<- conversation.Session
}

func (r *observedSessionCreations) CreateSession(ctx context.Context, request conversation.CreateSession) (conversation.Session, error) {
	session, err := r.Connection.CreateSession(ctx, request)
	if err == nil {
		select {
		case r.created <- session:
		case <-ctx.Done():
		}
	}
	return session, err
}
