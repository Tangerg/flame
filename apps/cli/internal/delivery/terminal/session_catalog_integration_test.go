package terminal

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestSessionCatalogRefreshKeepsItsSearchAndSelectedSession(t *testing.T) {
	connection, source := openSessionNavigationRuntime(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	create := func(title string) conversation.Session {
		t.Helper()
		created, err := connection.CreateSession(ctx, conversation.CreateSession{})
		if err != nil {
			t.Fatal(err)
		}
		if title == "" {
			return created
		}
		updated, err := connection.UpdateSession(ctx, conversation.UpdateSession{
			SessionID: created.ID, ExpectedRevision: created.Revision, Title: &title,
		})
		if err != nil {
			t.Fatal(err)
		}
		return updated
	}
	older := create("Navigation candidate A")
	newer := create("Navigation candidate B")
	for range conversation.DefaultPageRows {
		create("")
	}
	const incoming = "Navigation candidate C"
	captured := make(chan context.Context, 1)
	release := make(chan struct{})
	backend := &heldSessionCatalog{
		Connection: connection, title: incoming, captured: captured, release: release,
	}
	host := openSessionNavigationTerminal(t, connection, backend, source)
	if !host.Send(input.Key{Code: input.Character, Rune: 'r', Mods: input.Ctrl}) {
		t.Fatal("terminal refused to open Sessions")
	}
	host.Shows(t, "Sessions · Center")
	if !host.Send(input.Key{Code: input.Character, Rune: 'l', Mods: input.Alt}) {
		t.Fatal("terminal refused to load the next Session page")
	}
	host.Shows(t, "all sessions loaded")
	const search = "navigation candidate"
	if !host.Type(search) {
		t.Fatal("terminal refused to search loaded Sessions")
	}
	host.Shows(t, newer.ID)
	if !host.Press(input.Down) {
		t.Fatal("terminal refused to choose the older Session")
	}
	host.Shows(t, older.ID)
	create(incoming)
	awaitSessionRead(t, ctx, captured, "updated Session catalog")
	host.Shows(t, search)
	host.Shows(t, older.ID)
	close(release)
	host.Shows(t, incoming)
	host.Shows(t, search)
	host.Shows(t, older.ID)
	if !host.Send(input.Key{Code: input.Character, Rune: 'f', Mods: input.Alt}) {
		t.Fatal("terminal refused to favorite the selected Session")
	}
	host.Shows(t, "Favorites · "+older.Title)
	host.Shows(t, search)
	host.Shows(t, older.ID)
	if !host.Press(input.Enter) {
		t.Fatal("terminal refused to open the selected Session")
	}
	host.Shows(t, "session · "+older.Title)
}

type heldSessionCatalog struct {
	*runtimebinding.Connection
	title    string
	captured chan<- context.Context
	release  <-chan struct{}
}

func (r *heldSessionCatalog) ListSessions(ctx context.Context, query conversation.SessionQuery) (conversation.SessionPage, error) {
	page, err := r.Connection.ListSessions(ctx, query)
	if err != nil || !slices.ContainsFunc(page.Items, func(entry conversation.Session) bool { return entry.Title == r.title }) {
		return page, err
	}
	select {
	case r.captured <- ctx:
	default:
	}
	select {
	case <-r.release:
		return page, nil
	case <-ctx.Done():
		return conversation.SessionPage{}, context.Cause(ctx)
	}
}
