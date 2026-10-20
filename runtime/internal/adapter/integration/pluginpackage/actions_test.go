package pluginpackage

import (
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func TestSessionActionComponentAndRetiredActionAdmission(t *testing.T) {
	manifest := `{"$schema":"` + ManifestSchema + `","name":"test.actions","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`
	for _, test := range []struct {
		name, declaration string
		count             int
	}{
		{"component", `"components":[{"id":"rename","title":"Rename","scope":"session","placement":"sessionAction","entry":"io.github.tangerg.flame/components/component.js","export":"Rename"}]`, 1},
		{"retired", `"contributes":{"actions":[{"id":"rename","title":"Rename","operation":"renameSession"}]}`, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, map[string]string{
				"plugin.json": manifest, "io.github.tangerg.flame/declaration.json": flameDeclaration(test.declaration),
				"io.github.tangerg.flame/components/component.js": `throw new Error("admission must not evaluate action code");`,
			}))
			if err != nil {
				t.Fatal(err)
			}
			declaration := release.Declaration()
			if len(declaration.Components) != test.count || len(declaration.Servers) != 0 {
				t.Fatalf("admission: %+v", declaration)
			}
			if test.count == 1 && len(declaration.Diagnostics) != 0 {
				t.Fatalf("component refused: %+v", declaration)
			}
			if test.count == 0 && (len(declaration.Diagnostics) != 1 || declaration.Diagnostics[0].Code != plugin.DiagnosticInvalidDeclaration) {
				t.Fatalf("retired action admitted: %+v", declaration)
			}
		})
	}
}
