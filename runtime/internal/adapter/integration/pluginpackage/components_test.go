package pluginpackage

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

func TestSettingsIntentAdmissionDoesNotEvaluateTheComponent(t *testing.T) {
	for _, test := range []struct {
		name, placement, intent string
		admitted                bool
	}{
		{"model settings", "settingsPane", `,"settingsIntent":"modelProviders"`, true},
		{"empty", "settingsPane", `,"settingsIntent":""`, false},
		{"null", "settingsPane", `,"settingsIntent":null`, false},
		{"unknown", "settingsPane", `,"settingsIntent":"unknown"`, false},
		{"wrong placement", "workspaceView", `,"settingsIntent":"modelProviders"`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{
				"plugin.json": `{"$schema":"` + manifestSchema + `","name":"test.settings","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/components/component.js": `throw new Error("installation must not evaluate component code");`,
				"io.github.tangerg.flame/declaration.json":        flameDeclaration(`"components":[{"id":"settings","title":"Settings","scope":"client","placement":"` + test.placement + `","entry":"io.github.tangerg.flame/components/component.js","export":"Settings"` + test.intent + `}]`),
			}
			release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, files))
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			if test.admitted {
				if len(declaration.Components) != 1 || declaration.Components[0].SettingsIntent != extension.SettingsIntentModelProviders || len(declaration.Diagnostics) != 0 {
					t.Fatalf("settings intent: %+v", declaration)
				}
			} else if len(declaration.Components) != 0 || len(declaration.Diagnostics) != 1 {
				t.Fatalf("invalid settings intent admitted: %+v", declaration)
			}
		})
	}
}

func TestComponentOnlyPackageAdmissionDoesNotRequireOrExecuteBackend(t *testing.T) {
	for _, test := range []struct {
		name, placement, contribution string
		admitted                      bool
	}{
		{"absent", "workspaceView", "", true},
		{"settings", "settingsPane", "", true},
		{"empty", "workspaceView", `,"contribution":""`, false},
		{"unknown", "workspaceView", `,"contribution":"missing"`, false},
		{"invalid", "workspaceView", `,"contribution":"Invalid"`, false},
		{"null", "workspaceView", `,"contribution":null`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{
				"plugin.json": `{"$schema":"` + manifestSchema + `","name":"test.components","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/components/component.js": `throw new Error("installation must not evaluate component code");`,
				"io.github.tangerg.flame/declaration.json":        flameDeclaration(`"components":[{"id":"gallery","title":"Gallery","scope":"installation","placement":"` + test.placement + `","entry":"io.github.tangerg.flame/components/component.js","export":"Gallery"` + test.contribution + `}]`),
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

func TestComponentAdmissionPreservesTargetScopeAndNavigationKey(t *testing.T) {
	for _, test := range []struct {
		name, scope, material string
		admitted              bool
	}{
		{"valid", "workspace", `,"material":"diff"`, true},
		{"absent", "workspace", "", true},
		{"session", "session", "", true},
		{"client", "client", "", true},
		{"unsupported scope", "unknown", `,"material":"diff"`, false},
		{"invalid key", "workspace", `,"material":"Invalid"`, false},
		{"empty key", "workspace", `,"material":""`, false},
		{"null key", "workspace", `,"material":null`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := map[string]string{
				"plugin.json": `{"$schema":"` + manifestSchema + `","name":"test.workspace","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/components/component.js": `export const Workspace = () => null;`,
				"io.github.tangerg.flame/declaration.json":        flameDeclaration(`"components":[{"id":"workspace","title":"Workspace","scope":"` + test.scope + `","placement":"workspaceView","entry":"io.github.tangerg.flame/components/component.js","export":"Workspace"` + test.material + `}]`),
			}
			release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, files))
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			if test.admitted {
				if len(declaration.Components) != 1 || string(declaration.Components[0].Scope) != test.scope || (test.material != "" && declaration.Components[0].Material != "diff") || len(declaration.Diagnostics) != 0 {
					t.Fatalf("workspace metadata: %+v", declaration)
				}
			} else if len(declaration.Components) != 0 || len(declaration.Diagnostics) != 1 {
				t.Fatalf("invalid workspace metadata admitted: %+v", declaration)
			}
		})
	}
}

func TestFileComponentSelectorAdmissionDoesNotEvaluateCode(t *testing.T) {
	for _, test := range []struct {
		name, scope, selector string
		admitted              bool
	}{
		{"valid", "file", `{"type":"fileExtension","name":"md"}`, true},
		{"wrong scope", "workspace", `{"type":"fileExtension","name":"md"}`, false},
		{"missing", "file", `null`, false},
		{"wrong selector", "file", `{"type":"toolName","name":"read"}`, false},
		{"invalid extension", "file", `{"type":"fileExtension","name":"../md"}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := writePackage(t, map[string]string{
				"plugin.json": `{"$schema":"` + manifestSchema + `","name":"test.file","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
				"io.github.tangerg.flame/declaration.json":        flameDeclaration(`"components":[{"id":"file","title":"File","scope":"` + test.scope + `","placement":"fileRenderer","selector":` + test.selector + `,"entry":"io.github.tangerg.flame/components/component.js","export":"File"}]`),
				"io.github.tangerg.flame/components/component.js": `throw new Error("installation must not evaluate renderer code");`,
			})
			release, err := publishPackage(t.Context(), testReleases(t), source)
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			if test.admitted {
				if len(declaration.Components) != 1 || len(declaration.Diagnostics) != 0 || len(declaration.Servers) != 0 {
					t.Fatalf("file component: %+v", declaration)
				}
			} else if len(declaration.Components) != 0 || len(declaration.Diagnostics) != 1 {
				t.Fatalf("invalid file component admitted: %+v", declaration)
			}
		})
	}
}
