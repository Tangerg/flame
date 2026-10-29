package bootstrap

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

func TestIndependentComponentPackagesComposeThroughInstallationLifecycle(t *testing.T) {
	for _, first := range []string{"consumer", "provider"} {
		t.Run(first+" first", func(t *testing.T) {
			r := poMCPOpen(t, poMCPConfig(t))
			installed := map[string]*protocol.PluginInstallation{}
			sources := map[string]string{}
			install := func(name string) {
				t.Helper()
				source := t.TempDir()
				if err := os.CopyFS(source, os.DirFS(filepath.Join("../../../examples/plugins/composition", name, "package"))); err != nil {
					t.Fatal(err)
				}
				authorPluginPackage(t, source)
				sources[name] = source
				row := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-"+name).(*protocol.PluginInstallation)
				if len(row.Selected.Diagnostics) != 0 || len(row.Selected.Servers) != 0 {
					t.Fatalf("independent component admission: %+v", row.Selected)
				}
				installed[name] = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: row.ID, Enabled: true}, "enable-"+name).(*protocol.PluginInstallation)
			}
			request := func(name, point string, order ...protocol.PluginPointReference) protocol.ResolvePluginPointRequest {
				t.Helper()
				owner := installed[name]
				for _, declaration := range owner.Selected.Points {
					if declaration.ID == point {
						if order == nil {
							order = []protocol.PluginPointReference{}
						}
						return protocol.ResolvePluginPointRequest{Contract: declaration.Contract, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: owner.ID}, Attributes: `{}`, Order: order}
					}
				}
				t.Fatalf("missing point %s/%s", name, point)
				return protocol.ResolvePluginPointRequest{}
			}
			resolve := func(name, point string, want int) *protocol.Page[protocol.PluginPointBinding] {
				t.Helper()
				rows := r.must(delivery.PluginsResolvePoint, request(name, point), "").(*protocol.Page[protocol.PluginPointBinding])
				if len(rows.Data) != want {
					t.Fatalf("%s/%s has %d contributions, want %d", name, point, len(rows.Data), want)
				}
				return rows
			}
			install(first)
			if first == "consumer" {
				resolve("consumer", "panels", 0)
				install("provider")
			} else {
				resolve("provider", "details", 0)
				install("consumer")
			}
			consumer, provider := installed["consumer"], installed["provider"]
			panels := resolve("consumer", "panels", 1)
			if panels.Data[0].InstallationID != provider.ID || panels.Data[0].Digest != provider.Selected.Digest {
				t.Fatal("point lost the original component provider")
			}
			if resolve("provider", "details", 1).Data[0].InstallationID != consumer.ID {
				t.Fatal("reverse contribution was not independently installed")
			}
			frames := request("consumer", "frames", protocol.PluginPointReference{InstallationID: provider.ID, Contribution: "outer"}, protocol.PluginPointReference{InstallationID: provider.ID, Contribution: "inner"})
			ordered := r.must(delivery.PluginsResolvePoint, frames, "").(*protocol.Page[protocol.PluginPointBinding])
			if len(ordered.Data) != 2 || ordered.Data[0].Contribution != "outer" || ordered.Data[1].Contribution != "inner" {
				t.Fatal("consumer's decoration order was not preserved")
			}
			summary := request("consumer", "summary", protocol.PluginPointReference{InstallationID: provider.ID, Contribution: "summary"})
			if len(r.must(delivery.PluginsResolvePoint, summary, "").(*protocol.Page[protocol.PluginPointBinding]).Data) != 1 {
				t.Fatal("selection failed")
			}
			resource := protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: protocol.OpenPluginResourcesRequest{PluginReleaseRequest: protocol.PluginReleaseRequest{InstallationID: provider.ID, Digest: provider.Selected.Digest}, ComponentID: "panel"}, Path: "io.github.tangerg.flame/components/component.js"}
			r.must(delivery.PluginsReadResource, resource, "")
			refused := func(method delivery.Name, input any, want error) {
				t.Helper()
				result := r.endpoint.Invoke(t.Context(), method, input, delivery.Options{})
				if !errors.Is(result.Failure, want) {
					t.Fatalf("%s: got %v, want %v", method, result.Failure, want)
				}
			}
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: provider.ID}, "disable-provider")
			resolve("consumer", "panels", 0)
			refused(delivery.PluginsResolvePoint, summary, protocol.ErrPluginUnavailable)
			refused(delivery.PluginsReadResource, resource, protocol.ErrPluginDisabled)
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: provider.ID, Enabled: true}, "reenable-provider")
			resolve("consumer", "panels", 1)
			staged := stageCommandReplacement(t, r, provider, sources["provider"])
			if resolve("consumer", "panels", 1).Data[0].Digest != provider.Selected.Digest {
				t.Fatal("staging advanced selected membership")
			}
			selected := r.must(delivery.PluginsSelect, protocol.PluginReleaseRequest{InstallationID: provider.ID, Digest: staged.Staged.Digest}, "select-provider").(*protocol.PluginInstallation)
			if resolve("consumer", "panels", 1).Data[0].Digest != selected.Selected.Digest {
				t.Fatal("selected upgrade did not replace the binding")
			}
			refused(delivery.PluginsReadResource, resource, protocol.ErrPluginStale)
			resource.Digest = selected.Selected.Digest
			r.must(delivery.PluginsReadResource, resource, "")
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: consumer.ID}, "disable-consumer")
			refused(delivery.PluginsResolvePoint, request("consumer", "panels"), protocol.ErrPluginUnavailable)
			resolve("provider", "details", 0)
			r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: consumer.ID, Enabled: true}, "reenable-consumer")
			r.must(delivery.PluginsUninstall, protocol.PluginRequest{InstallationID: provider.ID}, "uninstall-provider")
			resolve("consumer", "panels", 0)
			refused(delivery.PluginsReadResource, resource, protocol.ErrPluginNotFound)
		})
	}
}
