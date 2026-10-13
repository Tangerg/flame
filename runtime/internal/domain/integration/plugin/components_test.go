package plugin

import "testing"

func TestSessionActionPlacementAdmission(t *testing.T) {
	for _, test := range []struct {
		name, scope, material string
		selector              *RendererSelector
		valid                 bool
	}{
		{name: "session", scope: "session", valid: true},
		{name: "client", scope: "client"},
		{name: "workspace", scope: "workspace"},
		{name: "material", scope: "session", material: "memory"},
		{name: "selector", scope: "session", selector: &RendererSelector{Kind: RendererToolName, Name: "read"}},
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
