package pluginpackage

import (
	"testing"
)

func TestComponentOnlyPackageAdmissionDoesNotRequireOrExecuteBackend(t *testing.T) {
	for _, test := range []struct {
		name, contribution string
		admitted           bool
	}{
		{"absent", "", true},
		{"empty", `,"contribution":""`, false},
		{"unknown", `,"contribution":"missing"`, false},
		{"invalid", `,"contribution":"Invalid"`, false},
		{"null", `,"contribution":null`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{
				"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.components","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/components/component.js": `throw new Error("installation must not evaluate component code");`,
				"io.github.tangerg.flame/declaration.json":        flameDeclaration(`"components":[{"id":"gallery","title":"Gallery","scope":"installation","placement":"workspaceView","entry":"io.github.tangerg.flame/components/component.js","export":"Gallery"` + test.contribution + `}]`),
			}
			release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, files))
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			if len(declaration.Servers) != 0 || len(declaration.Contributions) != 0 {
				t.Fatal("component-only package acquired a backend")
			}
			if test.admitted {
				if len(declaration.Components) != 1 || declaration.Components[0].Contribution != "" || len(declaration.Diagnostics) != 0 {
					t.Fatalf("component-only admission: %+v", declaration)
				}
			} else if len(declaration.Components) != 0 || len(declaration.Diagnostics) != 1 {
				t.Fatalf("invalid declared callable admitted: %+v", declaration)
			}
		})
	}
}

func TestWorkspaceComponentAdmissionPreservesScopeAndNavigationKey(t *testing.T) {
	for _, test := range []struct {
		name, scope, material string
		admitted              bool
	}{
		{"valid", "workspace", `,"material":"diff"`, true},
		{"absent", "workspace", "", true},
		{"unsupported scope", "session", `,"material":"diff"`, false},
		{"invalid key", "workspace", `,"material":"Invalid"`, false},
		{"empty key", "workspace", `,"material":""`, false},
		{"null key", "workspace", `,"material":null`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{
				"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.workspace","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/components/component.js": `export const Workspace = () => null;`,
				"io.github.tangerg.flame/declaration.json":        flameDeclaration(`"components":[{"id":"workspace","title":"Workspace","scope":"` + test.scope + `","placement":"workspaceView","entry":"io.github.tangerg.flame/components/component.js","export":"Workspace"` + test.material + `}]`),
			}
			release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, files))
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			if test.admitted {
				if len(declaration.Components) != 1 || declaration.Components[0].Scope != "workspace" || (test.material != "" && declaration.Components[0].Material != "diff") || len(declaration.Diagnostics) != 0 {
					t.Fatalf("workspace metadata: %+v", declaration)
				}
			} else if len(declaration.Components) != 0 || len(declaration.Diagnostics) != 1 {
				t.Fatalf("invalid workspace metadata admitted: %+v", declaration)
			}
		})
	}
}
