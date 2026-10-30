package plugin

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

func TestExportedComponentsHaveNoHostPlacementPolicy(t *testing.T) {
	for _, spelling := range extension.ComponentScope("").Values() {
		scope := extension.ComponentScope(spelling)
		t.Run(string(scope), func(t *testing.T) {
			builder, err := NewBuilder("exports", "", "")
			if err != nil {
				t.Fatal(err)
			}
			value := ClientComponent{ID: "panel", Title: "Panel", Scope: scope, Placement: "export", Entry: Namespace + "/components/component.js", Export: "Panel"}
			if err := builder.AdmitComponent(value); err != nil {
				t.Fatal(err)
			}
			value.ID = "material"
			value.Material = "memory"
			if err := builder.AdmitComponent(value); err == nil {
				t.Fatal("export acquired host navigation policy")
			}
			value.ID = "selector"
			value.Material = ""
			value.Selector = &RendererSelector{Kind: extension.RendererFileExtension, Name: "md"}
			if err := builder.AdmitComponent(value); err == nil {
				t.Fatal("export acquired host renderer policy")
			}
		})
	}
}

func TestSessionActionPlacementAdmission(t *testing.T) {
	for _, test := range []struct {
		name     string
		scope    extension.ComponentScope
		material string
		selector *RendererSelector
		valid    bool
	}{
		{name: "session", scope: "session", valid: true},
		{name: "client", scope: "client"},
		{name: "workspace", scope: "workspace"},
		{name: "material", scope: "session", material: "memory"},
		{name: "selector", scope: "session", selector: &RendererSelector{Kind: extension.RendererToolName, Name: "read"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder, err := NewBuilder("actions", "", "")
			if err != nil {
				t.Fatal(err)
			}
			err = builder.AdmitComponent(ClientComponent{ID: "rename", Title: "Rename", Scope: test.scope, Placement: "sessionAction", Material: test.material, Selector: test.selector, Entry: Namespace + "/components/component.js", Export: "Rename"})
			if (err == nil) != test.valid {
				t.Fatalf("admission: %v, valid %v", err, test.valid)
			}
		})
	}
}

func TestSettingsPanePlacementAdmitsOnlySettingsContexts(t *testing.T) {
	for _, test := range []struct {
		name     string
		scope    extension.ComponentScope
		selector *RendererSelector
		valid    bool
	}{
		{name: "client", scope: "client", valid: true},
		{name: "workspace", scope: "workspace", valid: true},
		{name: "installation", scope: "installation", valid: true},
		{name: "session", scope: "session"},
		{name: "file", scope: "file"},
		{name: "tool result", scope: "toolResult"},
		{name: "selector", scope: "client", selector: &RendererSelector{Kind: extension.RendererToolName, Name: "read"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder, err := NewBuilder("settings", "", "")
			if err != nil {
				t.Fatal(err)
			}
			err = builder.AdmitComponent(ClientComponent{ID: "settings", Title: "Provider settings", Scope: test.scope, Placement: "settingsPane", Material: "providers", Selector: test.selector, Entry: Namespace + "/components/component.js", Export: "Settings"})
			if (err == nil) != test.valid {
				t.Fatalf("admission: %v, valid %v", err, test.valid)
			}
		})
	}
}

func TestSettingsIntentRequiresADeclaredSettingsPane(t *testing.T) {
	for _, test := range []struct {
		name      string
		placement extension.ComponentPlacement
		intent    extension.SettingsIntent
		valid     bool
	}{
		{name: "mcp settings", placement: "settingsPane", intent: extension.SettingsIntentMCPServers, valid: true},
		{name: "model settings", placement: "settingsPane", intent: extension.SettingsIntentModelProviders, valid: true},
		{name: "ordinary settings", placement: "settingsPane", valid: true},
		{name: "unknown intent", placement: "settingsPane", intent: "invented"},
		{name: "workspace placement", placement: "workspaceView", intent: extension.SettingsIntentModelProviders},
		{name: "export placement", placement: "export", intent: extension.SettingsIntentModelProviders},
	} {
		t.Run(test.name, func(t *testing.T) {
			builder, err := NewBuilder("settings", "", "")
			if err != nil {
				t.Fatal(err)
			}
			err = builder.AdmitComponent(ClientComponent{ID: "settings", Title: "Model settings", Scope: "client", Placement: test.placement, SettingsIntent: test.intent, Entry: Namespace + "/components/component.js", Export: "Settings"})
			if (err == nil) != test.valid {
				t.Fatalf("admission: %v, valid %v", err, test.valid)
			}
		})
	}
}
