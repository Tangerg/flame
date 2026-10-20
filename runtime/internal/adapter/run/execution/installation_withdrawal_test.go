package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/resourceid"
	"github.com/Tangerg/scope/core/chat"
)

// liveCatalogTools resolves manifests from a live catalog that holds one
// installation release's tools until the installation owner withdraws them.
type liveCatalogTools struct {
	live atomic.Pointer[plugin.Dependency]
}

func (l *liveCatalogTools) Manifest(context.Context, domaintool.Group, []domaintool.Ref) (toolset.Manifest, error) {
	var manifest toolset.Manifest
	if dependency := l.live.Load(); dependency != nil {
		manifest.Installations = []plugin.Dependency{*dependency}
	}
	return identifyTestManifest(manifest)
}

type assemblingCatalogTools struct {
	catalog *liveCatalogTools
	entered chan struct{}
	proceed <-chan struct{}
}

func (a assemblingCatalogTools) Manifest(ctx context.Context, group domaintool.Group, capturedMCP []domaintool.Ref) (toolset.Manifest, error) {
	manifest, err := a.catalog.Manifest(ctx, group, capturedMCP)
	if err != nil || group != domaintool.GroupRoot {
		return manifest, err
	}
	close(a.entered)
	select {
	case <-a.proceed:
		return manifest, nil
	case <-ctx.Done():
		return toolset.Manifest{}, context.Cause(ctx)
	}
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
	mu      sync.Mutex
	record  *plugin.Record
	catalog releaseCatalog
}

func (s *singleInstallationStore) List(ctx context.Context) ([]plugin.Snapshot, error) {
	installation, found, err := s.Get(ctx, resourceid.InstallationID{})
	if err != nil || !found {
		return nil, err
	}
	return []plugin.Snapshot{installation}, err
}

func (s *singleInstallationStore) Get(ctx context.Context, _ resourceid.InstallationID) (plugin.Snapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.record == nil {
		return plugin.Snapshot{}, false, nil
	}
	selected, err := s.catalog.Get(ctx, s.record.Selected)
	if err != nil {
		return plugin.Snapshot{}, false, err
	}
	var staged *plugin.Release
	if s.record.Staged != nil {
		candidate, err := s.catalog.Get(ctx, *s.record.Staged)
		if err != nil {
			return plugin.Snapshot{}, false, err
		}
		staged = &candidate
	}
	installation, err := plugin.Restore(*s.record, selected, staged)
	if err != nil {
		return plugin.Snapshot{}, false, err
	}
	return plugin.Snapshot{Installation: installation, Selected: selected, Staged: staged}, true, nil
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

func (preparedPackages) ReadResource(context.Context, *plugin.Installation, string) ([]byte, error) {
	return nil, errors.New("unexpected component resource read")
}

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
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	store := &singleInstallationStore{catalog: releaseCatalog{release.Digest(): release}}
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

func TestWithdrawalRefusesAnUnpublishedRunWithTheFormerAuthority(t *testing.T) {
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package", Servers: []plugin.Server{{Name: testsupport.ServerName("server"), Transport: mcpserver.TransportStreamableHTTP, URL: "https://example.test/mcp"}}})
	installation, err := plugin.New(dependedInstallation, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	store := &singleInstallationStore{catalog: releaseCatalog{release.Digest(): release}}
	if err := store.Save(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	catalog := &liveCatalogTools{}
	catalog.live.Store(&plugin.Dependency{InstallationID: dependedInstallation, Digest: release.Digest()})
	entered, proceed := make(chan struct{}), make(chan struct{})
	finishAssembly := sync.OnceFunc(func() { close(proceed) })
	defer finishAssembly()
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("installation admission invoked the model")
	}), InteractionExecutorConfig{
		InstallationCheckpoints: pendingInstallationCheckpoints{},
		ToolResolver:            assemblingCatalogTools{catalog: catalog, entered: entered, proceed: proceed},
		ToolInterpreter:         testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	connections := realizingConnections{catalog: catalog, entered: make(chan struct{}), release: make(chan struct{})}
	close(connections.release)
	coordinator, err := plugins.New(t.Context(), store, store.catalog, preparedPackages{}, connections, executor, nil)
	if err != nil {
		t.Fatal(err)
	}
	assembled := make(chan error, 1)
	go func() {
		ref, err := executor.StageRoot(t.Context(), interactionTestStart())
		if err == nil {
			err = executor.Release(t.Context(), ref)
		}
		assembled <- err
	}()
	<-entered
	result, err := coordinator.Disable(t.Context(), dependedInstallation)
	if err != nil || result.View.Enabled == true {
		t.Fatalf("withdrawal = %+v, %v", result, err)
	}
	finishAssembly()
	if err := <-assembled; !errors.Is(err, runs.ErrInstallationChanged) {
		t.Fatalf("assembly admitted after disablement = %v, want installation-changed refusal", err)
	}
	if live := executor.sessions.snapshot(); len(live) != 0 {
		t.Fatalf("refused assembly retained %d owned sessions", len(live))
	}
}

func TestAssemblyPublicationFollowsCommittedExecutionChanges(t *testing.T) {
	for _, admission := range []plugins.ChangeAdmission{plugins.RequireQuiescent, plugins.AllowInUse} {
		for _, outcome := range []struct {
			name    string
			changed bool
			err     error
		}{
			{"committed", true, nil},
			{"withdrawal failed after commit", true, errors.New("withdrawal failed")},
			{"rejected before commit", false, errors.New("durable write failed")},
			{"unchanged", false, nil},
		} {
			t.Run(fmt.Sprintf("%d/%s", admission, outcome.name), func(t *testing.T) {
				executor := newInstallationTestExecutor(t, pendingInstallationCheckpoints{})
				assembly, abandon := executor.installations.Begin()
				defer abandon()
				if err := executor.ChangeInstallation(t.Context(), dependedInstallation, admission, func([]plugin.Dependency) (bool, error) {
					return outcome.changed, outcome.err
				}); !errors.Is(err, outcome.err) {
					t.Fatalf("change = %v, want %v", err, outcome.err)
				}
				published := false
				err := executor.installations.Publish(t.Context(), assembly, dependedRelease(), func() error {
					published = true
					return nil
				})
				if outcome.changed {
					if !errors.Is(err, runs.ErrInstallationChanged) || published {
						t.Fatalf("raced assembly = %v, published=%t; want installation-changed refusal", err, published)
					}
				} else if err != nil || !published {
					t.Fatalf("unchanged dependency refused publication = %v, published=%t", err, published)
				}
			})
		}
	}
}
