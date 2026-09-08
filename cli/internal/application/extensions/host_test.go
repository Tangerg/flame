package extensions

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func manifest(id string, setup func(*Scope) error) Plugin {
	return Plugin{ID: id, Version: "1.0.0", APIVersion: HostAPIVersion, Setup: setup}
}

func TestCapabilityProtectedPointDefaultsRestrictedPluginsToDeny(t *testing.T) {
	point := NewCapabilityKeyedPoint("test.command", Capability("commands"), func(value format) string { return value.ID })
	registry := new(Registry)
	denied := manifest("test.denied", func(scope *Scope) error {
		err := scope.Contribute(point, format{ID: "hello"}, Contribution{})
		return err
	})
	if _, err := Load(registry, denied); err == nil {
		t.Fatal("restricted plugin contributed without declaring the capability")
	}

	allowed := manifest("test.allowed", func(scope *Scope) error {
		err := scope.Contribute(point, format{ID: "hello"}, Contribution{})
		return err
	})
	allowed.Capabilities = []Capability{"commands"}
	loaded, err := Load(registry, allowed)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Dispose()
	if values := registry.Values(point); len(values) != 1 || values[0].ID != "hello" {
		t.Fatalf("values = %+v", values)
	}
}

func TestManifestValidationRejectsIncompatibleOrAmbiguousMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Plugin)
	}{
		{"id", func(plugin *Plugin) { plugin.ID = "Bad ID" }},
		{"version", func(plugin *Plugin) { plugin.Version = "latest" }},
		{"api", func(plugin *Plugin) { plugin.APIVersion++ }},
		{"self dependency", func(plugin *Plugin) { plugin.Requires = []string{plugin.ID} }},
		{"duplicate dependency", func(plugin *Plugin) { plugin.Requires = []string{"test.base", "test.base"} }},
		{"duplicate capability", func(plugin *Plugin) { plugin.Capabilities = []Capability{"commands", "commands"} }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := manifest("test.valid", func(*Scope) error { return nil })
			tt.mutate(&plugin)
			if err := ValidateManifest(plugin); err == nil {
				t.Fatalf("manifest was accepted: %+v", plugin)
			}
		})
	}
	invalid := Plugin{ID: "test.partial", Setup: func(*Scope) error { return nil }}
	if _, err := Load(new(Registry), invalid); err == nil {
		t.Fatal("Load bypassed full manifest validation")
	}
}

func TestHostRejectsDuplicatePluginIdentity(t *testing.T) {
	host, err := NewHost(new(Registry))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	plugin := manifest("test.duplicate", func(*Scope) error { return nil })
	results, err := host.Activate([]Plugin{plugin, plugin})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Phase != PluginSkipped || results[0].Err == nil {
		t.Fatalf("results = %+v", results)
	}
	statuses := host.Statuses()
	if len(statuses) != 1 || statuses[0].ID != plugin.ID || statuses[0].Phase != PluginSkipped {
		t.Fatalf("statuses = %+v", statuses)
	}
}

type failingSource struct{ err error }

func (failingSource) ID() string { return "broken" }
func (f failingSource) Discover(context.Context) (SourceResult, error) {
	return SourceResult{}, f.err
}

func TestDiscoveryPreservesSourceOrderAndIsolatesFailures(t *testing.T) {
	want := errors.New("offline")
	result, err := Discover(t.Context(),
		StaticSource{Name: "first", Plugins: []Plugin{manifest("test.first", func(*Scope) error { return nil })}},
		failingSource{err: want},
		StaticSource{Name: "last", Plugins: []Plugin{manifest("test.last", func(*Scope) error { return nil })}},
	)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{result.Plugins[0].ID, result.Plugins[1].ID}
	if !slices.Equal(ids, []string{"test.first", "test.last"}) {
		t.Fatalf("plugin order = %v", ids)
	}
	if len(result.Issues) != 1 || result.Issues[0].Source != "broken" || !errors.Is(result.Issues[0].Err, want) {
		t.Fatalf("issues = %+v", result.Issues)
	}
}

func TestHostOrdersDependenciesAndReloadsTheirClosure(t *testing.T) {
	registry := new(Registry)
	host, err := NewHost(registry)
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	point := newTestMultiPoint[string]("test.lifecycle")
	var lifecycle []string
	results, err := host.Activate([]Plugin{
		lifecyclePlugin(point, &lifecycle, "test.dependent", "test.base"),
		lifecyclePlugin(point, &lifecycle, "test.independent"),
		lifecyclePlugin(point, &lifecycle, "test.base"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !allLoaded(results) {
		t.Fatalf("activation = %+v", results)
	}
	requireLifecycle(t, lifecycle, []string{"load:test.independent", "load:test.base", "load:test.dependent"})
	requireAffected(t, host, "test.base", []string{"test.base", "test.dependent"})

	lifecycle = nil
	results, err = host.Reload("test.base")
	if err != nil || !allLoaded(results) {
		t.Fatalf("reload = %+v, %v", results, err)
	}
	requireLifecycle(t, lifecycle, []string{
		"load:test.base", "load:test.dependent",
	})
	if values := registry.Values(point); len(values) != 3 {
		t.Fatalf("reload left %d contributions, want 3", len(values))
	}
	if err := host.Unload("test.base"); err != nil {
		t.Fatal(err)
	}
	if values := registry.Values(point); !slices.Equal(values, []string{"test.independent"}) {
		t.Fatalf("unload retained dependent contributions: %v", values)
	}
	if results, err := host.Reload("test.base"); err != nil || !allLoaded(results) {
		t.Fatalf("reload after unload = %+v, %v", results, err)
	}
	if values := registry.Values(point); len(values) != 3 {
		t.Fatalf("reload after unload left contributions: %v", values)
	}
	host.Close()
	host.Close()
	if values := registry.Values(point); len(values) != 0 {
		t.Fatalf("close retained contributions: %v", values)
	}
}

func requireLifecycle(t *testing.T, got, want []string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("lifecycle = %v, want %v", got, want)
	}
}

func requireAffected(t *testing.T, host *Host, pluginID string, want []string) {
	t.Helper()
	got, err := host.Affected(pluginID)
	if err != nil {
		t.Fatal(err)
	}
	requireLifecycle(t, got, want)
}

func lifecyclePlugin(point Point[string], lifecycle *[]string, id string, requires ...string) Plugin {
	item := manifest(id, func(scope *Scope) error {
		*lifecycle = append(*lifecycle, "load:"+id)
		err := scope.Contribute(point, id, Contribution{})
		return err
	})
	item.Requires = requires
	return item
}

func TestHostSkipsMissingCyclesAndDependentsOfFailedSetup(t *testing.T) {
	broken := manifest("test.broken", func(*Scope) error { return errors.New("boom") })
	dependent := manifest("test.dependent", func(*Scope) error { return nil })
	dependent.Requires = []string{"test.broken"}
	missing := manifest("test.missing", func(*Scope) error { return nil })
	missing.Requires = []string{"test.absent"}
	cycleA := manifest("test.cycle-a", func(*Scope) error { return nil })
	cycleB := manifest("test.cycle-b", func(*Scope) error { return nil })
	cycleA.Requires = []string{cycleB.ID}
	cycleB.Requires = []string{cycleA.ID}

	host, err := NewHost(new(Registry))
	if err != nil {
		t.Fatal(err)
	}
	defer host.Close()
	results, err := host.Activate([]Plugin{dependent, broken, missing, cycleA, cycleB})
	if err != nil {
		t.Fatal(err)
	}
	phases := make(map[string]LifecyclePhase)
	for _, result := range results {
		phases[result.PluginID] = result.Phase
	}
	if phases[broken.ID] != PluginFailed || phases[dependent.ID] != PluginSkipped || phases[missing.ID] != PluginSkipped || phases[cycleA.ID] != PluginSkipped || phases[cycleB.ID] != PluginSkipped {
		t.Fatalf("phases = %+v", phases)
	}
}

func TestSetupPanicRollsBackContributionsAndReleasesPluginClaim(t *testing.T) {
	registry := new(Registry)
	point := newTestMultiPoint[string]("test.setup-panic")
	plugin := manifest("test.setup-panic", func(scope *Scope) error {
		if err := scope.Contribute(point, "owned", Contribution{}); err != nil {
			return err
		}
		panic("setup boom")
	})
	if _, err := Load(registry, plugin); err == nil {
		t.Fatal("setup panic escaped as success")
	}
	if values := registry.Values(point); len(values) != 0 {
		t.Fatalf("failed setup retained contributions: %v", values)
	}
	plugin.Setup = func(scope *Scope) error { return scope.Contribute(point, "replacement", Contribution{}) }
	loaded, err := Load(registry, plugin)
	if err != nil {
		t.Fatalf("failed setup retained the plugin claim: %v", err)
	}
	loaded.Dispose()
}

func TestHostDoesNotHoldStateLockWhileCallingPluginCode(t *testing.T) {
	host, err := NewHost(new(Registry))
	if err != nil {
		t.Fatal(err)
	}
	plugin := manifest("test.introspect", func(scope *Scope) error {
		_ = host.Statuses()
		return nil
	})
	done := make(chan error, 1)
	go func() {
		_, err := host.Activate([]Plugin{plugin})
		if err == nil {
			host.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("plugin setup deadlocked on host state")
	}
}

func allLoaded(results []LifecycleResult) bool {
	return len(results) > 0 && !slices.ContainsFunc(results, func(result LifecycleResult) bool { return result.Phase != PluginLoaded })
}
