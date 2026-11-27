package pluginpackage

import (
	json "encoding/json/v2"
	"errors"
	"strings"
	"testing"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func TestReleaseRequiresExactCompiledComponentRuntimeProfile(t *testing.T) {
	const directory = plugin.Namespace + "/components/"
	const entry = directory + "component.js"
	for _, item := range []struct {
		name    string
		profile string
	}{
		{name: "unversioned"},
		{name: "previous profile", profile: "io.github.tangerg.flame/component-runtime@1"},
		{name: "current profile", profile: hostModuleProfile},
	} {
		t.Run(item.name, func(t *testing.T) {
			graph, err := json.Marshal(moduleGraph{
				SchemaVersion: moduleGraphSchemaVersion, RuntimeProfile: item.profile, Entry: "component.js",
				Modules: []moduleEntry{{Path: "component.js", Exports: []string{"View"}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			source := writePackage(t, map[string]string{
				"plugin.json":                          `{"$schema":"` + manifestSchema + `","name":"test.component","extensions":{"` + plugin.Namespace + `":{"declaration":"` + plugin.Namespace + `/declaration.json"}}}`,
				plugin.Namespace + "/declaration.json": `{"protocol":"` + plugincontract.Protocol + `","components":[{"id":"view","title":"View","scope":"client","placement":"export","entry":"` + entry + `","export":"View"}]}`,
				entry:                                  "export const View = () => null;",
				directory + moduleGraphFilename:        string(graph),
			})
			releases := testReleases(t)
			release, err := publishPackage(t.Context(), releases, source)
			if item.profile != hostModuleProfile {
				if !errors.Is(err, plugin.ErrInvalid) || !strings.Contains(err.Error(), "requires rebuilding") {
					t.Fatalf("publish component runtime profile %q: %v", item.profile, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			components := release.Declaration().Components
			if len(components) != 1 {
				t.Fatalf("admitted components = %v", components)
			}
			profile, shared, err := releases.ComponentModuleImports(t.Context(), release, components[0])
			if err != nil || profile != hostModuleProfile || len(shared) != 0 {
				t.Fatalf("read component profile without shared imports = %q, %v, %v", profile, shared, err)
			}
		})
	}
}
