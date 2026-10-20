package runtime

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

func TestBundledUsageKeepsItsContractAtAnIndependentInstallationSource(t *testing.T) {
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_A2A_AGENTS", "")
	t.Setenv("FLAME_A2A_RPC_ORIGINS", "")
	cfg := Config{
		DataDirectory: testsupport.TempDirectory(t), UserHomePath: testsupport.TempDirectory(t),
		ConfigDirectories: []string{testsupport.TempDirectory(t)},
	}
	current, err := Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := current.Close(); err != nil {
			t.Error(err)
		}
	})
	installed, err := current.ListPlugins(t.Context(), protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var bundled protocol.PluginInstallation
	for _, candidate := range installed.Data {
		if candidate.Selected.Name == "flame.usage" {
			bundled = candidate
		}
	}
	if bundled.ID == "" || len(bundled.Selected.Components) != 1 {
		t.Fatalf("bundled Usage contract is absent: %+v", bundled)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	if err := os.CopyFS(source, os.DirFS(filepath.Join(filepath.Dir(executable), "plugins", "usage"))); err != nil {
		t.Fatal(err)
	}
	ordinary, err := current.InstallPlugin(t.Context(), protocol.InstallPluginRequest{Source: source}, protocol.CommandOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ordinary.ID == bundled.ID || ordinary.Source == bundled.Source || !reflect.DeepEqual(ordinary.Selected, bundled.Selected) {
		t.Fatalf("independent origin changed the contract or reused installation ownership: bundled=%+v ordinary=%+v", bundled, ordinary)
	}
	if _, err := current.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: ordinary.ID, Enabled: true}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	read := func(installation protocol.PluginInstallation) *protocol.PluginResource {
		t.Helper()
		resource, err := current.ReadPluginResource(t.Context(), protocol.ReadPluginResourceRequest{
			OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{
				PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: installation.ID, Digest: installation.Selected.Digest},
				ComponentID:          "usage",
			},
			Path: installation.Selected.Components[0].Entry,
		}, protocol.CallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return resource
	}
	expected := read(bundled)
	if actual := read(*ordinary); !reflect.DeepEqual(actual, expected) || actual.Body == "" {
		t.Fatalf("independent source did not serve the exact admitted resource: %+v", actual)
	}
	if _, err := current.SetPluginEnablement(t.Context(), protocol.SetPluginEnablementRequest{InstallationID: bundled.ID, Enabled: false}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
	if actual := read(*ordinary); !reflect.DeepEqual(actual, expected) {
		t.Fatal("withdrawing the bundled installation retargeted the independent resource")
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	current = reopened
	if actual := read(*ordinary); !reflect.DeepEqual(actual, expected) {
		t.Fatal("restart replaced the independent installation's admitted resource")
	}
	installed, err = current.ListPlugins(t.Context(), protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range installed.Data {
		if candidate.ID == bundled.ID && candidate.Enabled {
			t.Fatal("restart silently reenabled the withdrawn bundled installation")
		}
	}
	if err := current.UninstallPlugin(t.Context(), protocol.PluginRequest{InstallationID: ordinary.ID}, protocol.CommandOptions{}); err != nil {
		t.Fatal(err)
	}
}
