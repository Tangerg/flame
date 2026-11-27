package terminal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/runtimebinding"
	"github.com/Tangerg/flame/apps/cli/internal/application/settings"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/apps/cli/internal/runtimefixture"
	"github.com/Tangerg/oolong/ui/core/input"
)

func TestTerminalForkSupersedesTheSourceSessionRefresh(t *testing.T) {
	const forkTitle = "Navigation target"
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
	gotRefresh := make(chan context.Context, 1)
	gotFork := make(chan context.Context, 1)
	releaseRefresh := make(chan struct{})
	releaseFork := make(chan struct{})
	backend := &heldNavigationReads{
		Connection: connection, source: source, forkTitle: forkTitle,
		refresh: heldSessionRead{captured: gotRefresh, release: releaseRefresh},
		fork:    heldSessionRead{captured: gotFork, release: releaseFork},
	}
	profile := connection.Profile()
	host, stop, _ := runUIFromConfig(t, Config{
		Runtime: backend, RuntimeProfile: &profile, Workspace: workspace,
		Workspaces: connection, Changes: connection, Transfers: connection, Usage: connection, ModelConfig: connection,
		Goals: connection, Skills: connection, MCP: connection, Plugins: connection,
		DiagnosticTools: connection.DiagnosticTools(), AuthoringContext: connection.AuthoringContext(),
		Hooks: connection.Hooks(), Feedback: connection.Feedback(),
		SessionID: source.ID, OpenWorkbench: func() (*workbench.Store, error) { return workbench.OpenMemory(workbench.Config{}) },
		Settings: settings.Default(),
	})
	host.Shows(t, "Ask flame")
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
	stop()
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
