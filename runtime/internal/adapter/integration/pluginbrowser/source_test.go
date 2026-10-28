package pluginbrowser

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type sourceStore struct{ snapshot plugin.Snapshot }

func (s *sourceStore) Get(context.Context, identity.InstallationID) (plugin.Snapshot, bool, error) {
	return s.snapshot, true, nil
}
func (s *sourceStore) List(context.Context) ([]plugin.Snapshot, error) {
	return []plugin.Snapshot{s.snapshot}, nil
}
func (*sourceStore) Stage(context.Context, identity.InstallationID, identity.Digest) error {
	return errors.New("fixture must not mutate installations")
}

type sourceResources struct {
	body  []byte
	reads int
}

func (*sourceResources) ComponentModules(context.Context, plugin.Release, plugin.ContributionID) (*distribution.Module, error) {
	return &distribution.Module{Entry: plugin.Namespace + "/component.js", Imports: []distribution.ModuleImport{}}, nil
}

func (s *sourceResources) ReadResource(_ context.Context, _ *plugin.Installation, path string) ([]byte, error) {
	if path != plugin.Namespace+"/component.js" {
		return nil, errors.New("unexpected resource")
	}
	s.reads++
	return bytes.Clone(s.body), nil
}

func sourceFixture(t *testing.T) (*Source, *sourceStore, *sourceResources, qimu.UnitBinding) {
	t.Helper()
	digest, err := identity.ParseDigest(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	id, err := identity.ParseInstallation("93d2cde5-4676-4375-9e74-afd6b173e2a3")
	if err != nil {
		t.Fatal(err)
	}
	declaration := plugin.Declaration{Name: "native-component", Components: []plugin.ClientComponent{{ID: "panel", Title: "Panel", Scope: "installation", Placement: "export", Entry: plugin.Namespace + "/component.js", Export: "Panel"}}}
	release, err := plugin.NewRelease(digest, declaration)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(id, "/private/install/source", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := installation.Enable(release); err != nil {
		t.Fatal(err)
	}
	store := &sourceStore{snapshot: plugin.Snapshot{Installation: installation, Selected: release}}
	resources := &sourceResources{body: []byte("export const Panel = () => null;")}
	source, err := NewSource(store, resources)
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := plugins.EncodeFrameworkConfiguration(installation.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	binding := qimu.UnitBinding{Source: qimu.UnitRef{InstallationID: qimu.InstallationID(id.String()), Release: qimu.Digest(digest.String()), UnitID: plugins.ComponentUnit("panel")}, Configuration: qimu.Hash(configuration), Imports: []qimu.ImportDeclaration{}}
	return source, store, resources, binding
}

func TestComponentArtifactUsesExactAdmittedBytesAndRefusesChangedConfiguration(t *testing.T) {
	source, store, resources, binding := sourceFixture(t)
	artifact, err := source.Module(t.Context(), binding)
	if err != nil || artifact.Path != plugin.Namespace+"/component.js" || artifact.Digest != qimu.Hash(resources.body) {
		t.Fatalf("artifact = %+v, %v", artifact, err)
	}
	if resources.reads != 1 {
		t.Fatal("module did not use the original verified resource owner")
	}
	changed := binding.Clone()
	changed.Configuration = qimu.Hash([]byte("different configuration"))
	if _, err := source.Module(t.Context(), changed); !errors.Is(err, qimu.ErrStaleBinding) {
		t.Fatalf("changed configuration = %v", err)
	}
	changed = binding.Clone()
	changed.Source.Release = qimu.Hash([]byte("different release"))
	if _, err := source.Module(t.Context(), changed); !errors.Is(err, plugin.ErrStale) {
		t.Fatalf("changed release = %v", err)
	}
	store.snapshot.Installation.Disable()
	if _, err := source.Module(t.Context(), binding); !errors.Is(err, plugin.ErrDisabled) {
		t.Fatalf("disabled installation = %v", err)
	}
	if resources.reads != 1 {
		t.Fatal("refused bindings opened content")
	}
}

type configurationDriver struct {
	source   *Source
	observed []byte
}

func (*configurationDriver) Profile() qimu.Profile { return qimu.BrowserJS }
func (d *configurationDriver) Prepare(ctx context.Context, setup *qimu.Setup) ([]qimu.ExportValue, error) {
	var err error
	d.observed, err = d.source.Configuration(ctx, setup)
	if err != nil {
		return nil, err
	}
	contract, err := plugins.ComponentContract()
	if err != nil {
		return nil, err
	}
	return []qimu.ExportValue{qimu.Provide[any]("component", contract, struct{}{})}, nil
}

func TestBrowserConfigurationIsThePublicProjectionOfTheApprovedUnit(t *testing.T) {
	source, store, _, binding := sourceFixture(t)
	driver := &configurationDriver{source: source}
	host, err := qimu.NewHost(t.Context(), qimu.HostConfig{Drivers: []qimu.Driver{driver}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		outcome, err := host.Close(context.Background())
		if err != nil || !outcome.Released {
			t.Errorf("host close = %+v, %v", outcome, err)
		}
	})
	release, err := plugins.ProjectFrameworkRelease(store.snapshot.Selected)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.RegisterFrameworkRelease(host, release); err != nil {
		t.Fatal(err)
	}
	configuration, err := plugins.EncodeFrameworkConfiguration(store.snapshot.Installation.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Install(t.Context(), qimu.InstallationConfig{ID: binding.Source.InstallationID, Plugin: release.Plugin, Configuration: configuration, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	contract, err := plugins.ComponentContract()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := host.Activate(t.Context(), qimu.ExportRef{InstallationID: binding.Source.InstallationID, Release: binding.Source.Release, UnitID: binding.Source.UnitID, Export: "component", Contract: contract.Ref()}); err != nil {
		t.Fatal(err)
	}
	expected, err := plugins.PublicComponentConfiguration(store.snapshot.Selected, "panel")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(driver.observed, expected) || bytes.Contains(driver.observed, []byte("/private/install/source")) || bytes.Contains(driver.observed, []byte("values")) {
		t.Fatalf("browser received private or invented setup configuration: %s", driver.observed)
	}
}
