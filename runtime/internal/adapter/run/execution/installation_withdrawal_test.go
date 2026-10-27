package execution

import (
	"context"
	"errors"
	"github.com/Tangerg/flame/qimu"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/pluginhost"
	"github.com/Tangerg/scope/core/chat"
)

// liveCatalogTools resolves manifests from a live catalog that holds one
// installation release's tools until the installation owner withdraws them.
type liveCatalogTools struct {
	live atomic.Pointer[plugin.Dependency]
}

func (l *liveCatalogTools) Manifest(context.Context, domaintool.Group, *toolset.Retained) (toolset.Manifest, error) {
	var manifest toolset.Manifest
	if dependency := l.live.Load(); dependency != nil {
		manifest.Installations = []plugin.Dependency{*dependency}
		manifest.Resources = []plugin.Dependency{*dependency}
	}
	return identifyTestManifest(manifest)
}

type assemblingCatalogTools struct {
	catalog *liveCatalogTools
	entered chan struct{}
	proceed <-chan struct{}
}

func (a assemblingCatalogTools) Manifest(ctx context.Context, group domaintool.Group, retained *toolset.Retained) (toolset.Manifest, error) {
	manifest, err := a.catalog.Manifest(ctx, group, retained)
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

func withdrawalStore(t *testing.T, release plugin.Release, installation *plugin.Installation) (*persistence.Bundle, *pluginhost.Fixture) {
	t.Helper()
	stores, err := persistence.Open(t.Context(), persistence.Config{DataDirectory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stores.Close() })
	if err := stores.PluginReleases.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	framework := pluginhost.New(t, stores)
	if err := framework.Apply(t.Context(), installation); err != nil {
		t.Fatal(err)
	}
	return stores, framework
}

type preparedPackages struct{}

func (preparedPackages) ReadResource(context.Context, *plugin.Installation, string) ([]byte, error) {
	return nil, errors.New("unexpected component resource read")
}

func (preparedPackages) Materialize(context.Context, string) (plugins.Candidate, error) {
	return nil, errors.New("unexpected materialization")
}
func (preparedPackages) Reclaim(context.Context) error { return nil }
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
	stores, framework := withdrawalStore(t, release, installation)
	catalog := &liveCatalogTools{}
	catalog.live.Store(&plugin.Dependency{InstallationID: dependedInstallation, Digest: release.Digest()})
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("installation admission invoked the model")
	}), InteractionExecutorConfig{
		Admission:       stores.Admission,
		ToolResolver:    catalog,
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	connections := realizingConnections{catalog: catalog, entered: make(chan struct{}), release: make(chan struct{})}
	if err := stores.Admission.RegisterUseOwner(executor); err != nil {
		t.Fatal(err)
	}
	coordinator, err := plugins.New(t.Context(), framework.Host, stores.Installations, stores.PluginReleases, preparedPackages{}, connections, nil)
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
	stores, framework := withdrawalStore(t, release, installation)
	catalog := &liveCatalogTools{}
	catalog.live.Store(&plugin.Dependency{InstallationID: dependedInstallation, Digest: release.Digest()})
	entered, proceed := make(chan struct{}), make(chan struct{})
	finishAssembly := sync.OnceFunc(func() { close(proceed) })
	defer finishAssembly()
	executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		return nil, errors.New("installation admission invoked the model")
	}), InteractionExecutorConfig{
		Admission:       stores.Admission,
		ToolResolver:    assemblingCatalogTools{catalog: catalog, entered: entered, proceed: proceed},
		ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
	})
	connections := realizingConnections{catalog: catalog, entered: make(chan struct{}), release: make(chan struct{})}
	close(connections.release)
	if err := stores.Admission.RegisterUseOwner(executor); err != nil {
		t.Fatal(err)
	}
	coordinator, err := plugins.New(t.Context(), framework.Host, stores.Installations, stores.PluginReleases, preparedPackages{}, connections, nil)
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
	if err := <-assembled; !errors.Is(err, qimu.ErrStaleBinding) {
		t.Fatalf("assembly admitted after disablement = %v, want installation-changed refusal", err)
	}
	if live := executor.sessions.snapshot(); len(live) != 0 {
		t.Fatalf("refused assembly retained %d owned sessions", len(live))
	}
}

func TestAssemblyPublicationFollowsCommittedExecutionChanges(t *testing.T) {
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
		t.Run(outcome.name, func(t *testing.T) {
			executor := newInstallationTestExecutor(t)
			assembly, abandon, err := executor.installations.Begin()
			if err != nil {
				t.Fatal(err)
			}
			defer abandon()
			if err := executor.installations.Change(t.Context(), qimu.InstallationID(dependedInstallation.String()), func() (bool, error) {
				return outcome.changed, outcome.err
			}); !errors.Is(err, outcome.err) {
				t.Fatalf("change = %v, want %v", err, outcome.err)
			}
			published := false
			err = executor.installations.Publish(t.Context(), assembly, dependencyInstallations(dependedRelease()), func() error {
				published = true
				return nil
			})
			if outcome.changed {
				if !errors.Is(err, qimu.ErrStaleBinding) || published {
					t.Fatalf("raced assembly = %v, published=%t; want installation-changed refusal", err, published)
				}
			} else if err != nil || !published {
				t.Fatalf("unchanged dependency refused publication = %v, published=%t", err, published)
			}
		})
	}
}
