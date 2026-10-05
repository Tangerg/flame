package execution

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

// liveCatalogTools resolves manifests from a live catalog that holds one
// installation release's tools until the installation owner withdraws them.
type liveCatalogTools struct {
	live atomic.Pointer[plugin.Dependency]
}

func (l *liveCatalogTools) Manifest(context.Context, domaintool.Group) (toolset.Manifest, error) {
	var manifest toolset.Manifest
	if dependency := l.live.Load(); dependency != nil {
		manifest.Installations = []plugin.Dependency{*dependency}
	}
	return identifyTestManifest(manifest)
}

// realizingConnections withdraws into the live catalog and holds the
// realization that follows the change, so a test can assemble in that window.
type realizingConnections struct {
	catalog *liveCatalogTools
	entered chan struct{}
	release chan struct{}
}

func (c realizingConnections) WithdrawInstallation([]mcpserver.ID) error {
	c.catalog.live.Store(nil)
	return nil
}

func (c realizingConnections) ReconcileInstallation(ctx context.Context, _ []mcpserver.ID) error {
	close(c.entered)
	select {
	case <-c.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

type singleInstallationStore struct {
	mu     sync.Mutex
	record *plugin.Record
}

func (s *singleInstallationStore) List(ctx context.Context) ([]*plugin.Installation, error) {
	installation, err := s.Get(ctx, resourceid.InstallationID{})
	if errors.Is(err, plugin.ErrNotFound) {
		return nil, nil
	}
	return []*plugin.Installation{installation}, err
}

func (s *singleInstallationStore) Get(context.Context, resourceid.InstallationID) (*plugin.Installation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil {
		return nil, plugin.ErrNotFound
	}
	return plugin.Restore(*s.record)
}

func (s *singleInstallationStore) Save(_ context.Context, installation *plugin.Installation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := installation.Snapshot()
	s.record = &record
	return nil
}

func (s *singleInstallationStore) Remove(context.Context, resourceid.InstallationID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.record = nil
	return nil
}

type releaseCatalog map[fingerprint.Digest]plugin.Release

func (c releaseCatalog) Get(_ context.Context, digest fingerprint.Digest) (plugin.Release, error) {
	release, found := c[digest]
	if !found {
		return plugin.Release{}, plugin.ErrNotFound
	}
	return release, nil
}

type preparedPackages struct{}

func (preparedPackages) Materialize(context.Context, string) (plugins.Candidate, error) {
	return nil, errors.New("unexpected materialization")
}
func (preparedPackages) Reclaim(context.Context, []fingerprint.Digest) error { return nil }
func (preparedPackages) Prepare(context.Context, *plugin.Installation, plugin.Release) error {
	return nil
}
func (preparedPackages) Realize(context.Context, *plugin.Installation, plugin.Release) (plugins.Realization, error) {
	return plugins.Realization{Release: plugins.ReleaseAvailable}, nil
}

// H1: the live tool catalog follows an installation change inside the
// admission critical section. A Run assembled after the commit, while the
// change is still realizing its backends, cannot freeze the removed release's
// tools, so the removal never leaves a session depending on it.
func TestRunAssembledAfterUninstallCommitCannotDependOnTheRemovedRelease(t *testing.T) {
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp"}}})
	installation, err := plugin.New(dependedInstallation, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Approve(release); err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	store := &singleInstallationStore{}
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	catalog := &liveCatalogTools{}
	catalog.live.Store(&plugin.Dependency{InstallationID: dependedInstallation, Digest: release.Digest()})
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("installation admission invoked the model")
	}), InteractionExecutorConfig{
		InstallationCheckpoints: pendingInstallationCheckpoints{},
		ToolResolver:            catalog,
		ToolInterpreter:         testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	connections := realizingConnections{catalog: catalog, entered: make(chan struct{}), release: make(chan struct{})}
	coordinator, err := plugins.New(t.Context(), store, releaseCatalog{release.Digest(): release}, preparedPackages{}, connections, executor, nil)
	if err != nil {
		t.Fatal(err)
	}
	uninstalled := make(chan error, 1)
	go func() { uninstalled <- coordinator.Uninstall(t.Context(), dependedInstallation) }()
	<-connections.entered

	ref, err := executor.StageRoot(t.Context(), interactionTestStart())
	if err != nil {
		close(connections.release)
		t.Fatal(err)
	}
	session, err := executor.session(ref)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(session.installationDependencies(), func(dependency plugin.Dependency) bool {
		return dependency.InstallationID == dependedInstallation
	}) {
		t.Error("a Run assembled after the removal committed froze the removed release's tools")
	}
	if err := executor.Release(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	close(connections.release)
	if err := <-uninstalled; err != nil {
		t.Fatal(err)
	}
}

// A change may fail after it committed and withdrew; the assemblies it raced
// are refused all the same, with the retryable category rather than a release
// conflict the client could not act on.
func TestFailedChangeStillRefusesTheAssembliesItRaced(t *testing.T) {
	executor := newInstallationTestExecutor(t, pendingInstallationCheckpoints{})
	assembly, abandon := executor.installations.begin()
	defer abandon()
	committedThenFailed := errors.New("withdrawal failed after the durable write")
	if err := executor.ChangeInstallation(t.Context(), dependedInstallation, plugins.RequireQuiescent, func([]plugin.Dependency) error {
		return committedThenFailed
	}); !errors.Is(err, committedThenFailed) {
		t.Fatalf("change = %v", err)
	}
	published := false
	err := executor.installations.publish(t.Context(), assembly, dependedRelease(), func() error {
		published = true
		return nil
	})
	if !errors.Is(err, runs.ErrInstallationChanged) || published {
		t.Fatalf("raced assembly = %v, published=%t; want installation-changed refusal", err, published)
	}
}
