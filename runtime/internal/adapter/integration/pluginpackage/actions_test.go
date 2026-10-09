package pluginpackage

import "testing"

func TestActionAdmissionIsolatesUnsupportedAndMalformedSiblings(t *testing.T) {
	manifest := `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"test.actions","extensions":{"io.github.tangerg.flame":{"apiVersion":1,"contributes":{"actions":[{"id":"rename","title":"Rename","operation":"renameSession"},{"id":"rpc","title":"RPC","operation":"tools.invoke"},{"id":"schema","title":"Schema","operation":"renameSession","schema":{}},{"id":"rename","title":"Duplicate","operation":"renameSession"},null]}}}}`
	release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, map[string]string{"plugin.json": manifest}))
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	if len(declaration.Actions) != 1 || declaration.Actions[0].ID != "rename" || len(declaration.Diagnostics) != 4 {
		t.Fatalf("admission: %+v", declaration)
	}
}
