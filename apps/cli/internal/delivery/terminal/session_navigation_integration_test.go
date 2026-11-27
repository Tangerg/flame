package terminal

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/oolong/ui/core/input"
	"github.com/Tangerg/oolong/ui/uitest"
)

func TestTerminalForkSupersedesTheSourceSessionRefresh(t *testing.T) {
	const forkTitle = "Navigation target"
	connection, source := openSessionNavigationRuntime(t)
	gotRefresh := make(chan context.Context, 1)
	gotFork := make(chan context.Context, 1)
	releaseRefresh := make(chan struct{})
	releaseFork := make(chan struct{})
	backend := &heldNavigationReads{
		Connection: connection, source: source, forkTitle: forkTitle,
		refresh: heldSessionRead{captured: gotRefresh, release: releaseRefresh},
		fork:    heldSessionRead{captured: gotFork, release: releaseFork},
	}
	host := openSessionNavigationTerminal(t, connection, backend, source)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	relocated := t.TempDir()
	updated, err := connection.UpdateSession(ctx, conversation.UpdateSession{
		SessionID: source.ID, Workspace: &relocated, ExpectedRevision: source.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Workspace == source.Workspace {
		t.Fatal("Runtime kept the original workspace")
	}
	refreshCtx := awaitSessionRead(t, ctx, gotRefresh, "source refresh")
	if !host.Type("/fork "+forkTitle) || !host.Press(input.Enter) {
		t.Fatal("terminal refused the fork command")
	}
	forkCtx := awaitSessionRead(t, ctx, gotFork, "fork snapshot")
	select {
	case <-refreshCtx.Done():
		close(releaseRefresh)
	default:
		close(releaseRefresh)
		select {
		case <-forkCtx.Done():
			t.Fatal("the previous Session refresh canceled navigation to the committed fork")
		case <-ctx.Done():
			t.Fatal("the previous Session refresh remained active after navigation began")
		}
	}
	close(releaseFork)
	host.Shows(t, "session · "+forkTitle)
}

func TestTerminalCanOpenTheCommittedForkAfterOpeningStops(t *testing.T) {
	t.Run("snapshot fails", func(t *testing.T) { verifyCommittedForkOpening(t, false) })
	t.Run("opening canceled", func(t *testing.T) { verifyCommittedForkOpening(t, true) })
}

func verifyCommittedForkOpening(t *testing.T, cancelOpening bool) {
	t.Helper()
	const title = "Recovery target"
	connection, source := openSessionNavigationRuntime(t)
	captured := make(chan string, 1)
	release := make(chan struct{})
	backend := &unavailableForkSnapshot{Connection: connection, title: title, captured: captured, release: release}
	host := openSessionNavigationTerminal(t, connection, backend, source)
	if !host.Type("/fork "+title) || !host.Press(input.Enter) {
		t.Fatal("terminal refused the fork command")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	var committed string
	select {
	case committed = <-captured:
	case <-ctx.Done():
		t.Fatal("terminal did not read the committed fork", context.Cause(ctx))
	}
	acknowledgement := "forked session · " + title
	host.Shows(t, acknowledgement+" · opening")
	if cancelOpening {
		if !host.Send(input.Key{Code: input.Character, Rune: 'c', Mods: input.Ctrl}) {
			t.Fatal("terminal refused to cancel opening the fork")
		}
		host.Shows(t, acknowledgement+" · opening canceled")
	} else {
		close(release)
		host.Shows(t, acknowledgement+" · could not open")
	}
	if !host.Type("/sessions") || !host.Press(input.Enter) {
		t.Fatal("terminal refused to open Sessions")
	}
	host.Shows(t, "Sessions · Center")
	if !host.Type(title) {
		t.Fatal("terminal refused to search Sessions")
	}
	host.Shows(t, committed)
	if !host.Press(input.Enter) {
		t.Fatal("terminal refused to open the original fork")
	}
	host.Shows(t, "session · "+title)
	page, err := connection.ListSessions(ctx, conversation.SessionQuery{Search: title, PageSize: conversation.DefaultPageSize()})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(page.Items))
	for _, found := range page.Items {
		ids = append(ids, found.ID)
	}
	if !slices.Equal(ids, []string{committed}) {
		t.Fatalf("committed fork %s has unexpected catalog entries %v", committed, ids)
	}
}

func openSessionNavigationRuntime(t *testing.T) (*runtimebinding.Connection, conversation.Session) {
	t.Helper()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("session navigation called the model provider")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", "deepseek-chat")
	t.Setenv("FLAME_APIKEY", "integration-key")
	t.Setenv("FLAME_BASEURL", provider.URL)
	workspace := t.TempDir()
	owner := runtimebinding.NewOwner(runtimebinding.Config{
		ProductRoot: runtimefixture.TempDirectory(t), DefaultWorkspacePath: workspace,
		UserHomePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}, ClientVersion: "test",
	})
	t.Cleanup(func() {
		if err := owner.Close(); err != nil {
			t.Error(err)
		}
	})
	connection, err := owner.Connection(t.Context(), settings.Runtime{Mode: settings.RuntimeEmbedded})
	if err != nil {
		t.Fatal(err)
	}
	source, err := connection.CreateSession(t.Context(), conversation.CreateSession{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	return connection, source
}

func openSessionNavigationTerminal(t *testing.T, connection *runtimebinding.Connection, backend Runtime, source conversation.Session) *uitest.Host {
	t.Helper()
	host, _, _ := runUIFromConfig(t, sessionNavigationConfig(connection, backend, source))
	host.Shows(t, "Ask flame")
	return host
}

func sessionNavigationConfig(connection *runtimebinding.Connection, backend Runtime, source conversation.Session) Config {
	profile := connection.Profile()
	return Config{
		Runtime: backend, RuntimeProfile: &profile, Workspace: source.Workspace.Path,
		Workspaces: connection, Changes: connection, Transfers: connection, Usage: connection, ModelConfig: connection,
		Goals: connection, Skills: connection, MCP: connection, Plugins: connection,
		DiagnosticTools: connection.DiagnosticTools(), AuthoringContext: connection.AuthoringContext(),
		Hooks: connection.Hooks(), Feedback: connection.Feedback(),
		SessionID: source.ID, OpenWorkbench: func() (*workbench.Store, error) { return workbench.OpenMemory(workbench.Config{}) },
		Settings: settings.Default(),
	}
}

type unavailableForkSnapshot struct {
	*runtimebinding.Connection
	title    string
	captured chan<- string
	release  <-chan struct{}
	injected atomic.Bool
}

func (r *unavailableForkSnapshot) GetSession(ctx context.Context, id string) (conversation.SessionSnapshot, error) {
	snapshot, err := r.Connection.GetSession(ctx, id)
	if err == nil && snapshot.Session.Title == r.title && r.injected.CompareAndSwap(false, true) {
		r.captured <- id
		select {
		case <-r.release:
			return conversation.SessionSnapshot{}, errors.New("snapshot unavailable")
		case <-ctx.Done():
			return conversation.SessionSnapshot{}, context.Cause(ctx)
		}
	}
	return snapshot, err
}

type heldNavigationReads struct {
	*runtimebinding.Connection
	source    conversation.Session
	forkTitle string
	refresh   heldSessionRead
	fork      heldSessionRead
}

func (r *heldNavigationReads) GetSession(ctx context.Context, id string) (conversation.SessionSnapshot, error) {
	snapshot, err := r.Connection.GetSession(ctx, id)
	if err != nil {
		return snapshot, err
	}
	switch {
	case id == r.source.ID && snapshot.Session.Workspace != r.source.Workspace:
		return r.refresh.wait(ctx, snapshot)
	case snapshot.Session.Title == r.forkTitle:
		return r.fork.wait(ctx, snapshot)
	default:
		return snapshot, nil
	}
}

type heldSessionRead struct {
	captured chan<- context.Context
	release  <-chan struct{}
}

func (r heldSessionRead) wait(ctx context.Context, snapshot conversation.SessionSnapshot) (conversation.SessionSnapshot, error) {
	select {
	case r.captured <- ctx:
	default:
	}
	select {
	case <-r.release:
		return snapshot, nil
	case <-ctx.Done():
		return conversation.SessionSnapshot{}, context.Cause(ctx)
	}
}

func awaitSessionRead(t *testing.T, ctx context.Context, reads <-chan context.Context, label string) context.Context {
	t.Helper()
	select {
	case captured := <-reads:
		return captured
	case <-ctx.Done():
		t.Fatalf("terminal did not read %s: %v", label, context.Cause(ctx))
		return nil
	}
}
