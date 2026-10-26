package pluginpackage

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/point"
)

func TestPackageAdmitsLocalPointContractsAndIsolatesInvalidValues(t *testing.T) {
	contract, err := point.Admit(point.Definition{Protocol: capability.Protocol, Reference: "example.com/analyzers", Composition: capability.Collection, Scope: "installation", Optional: true, Ordering: point.CanonicalKeys, Value: jsontext.Value(`{"type":"object","required":["title"],"properties":{"title":{"type":"string"}},"additionalProperties":false}`), Applicability: jsontext.Value(`{"type":"object"}`)})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(contract.Definition())
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"plugin.json": `{"$schema":"` + ManifestSchema + `","name":"test.points","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
		"io.github.tangerg.flame/contracts/point.json": string(data),
		"io.github.tangerg.flame/declaration.json":     flameDeclaration(`"points":[{"id":"analyzers","contract":"io.github.tangerg.flame/contracts/point.json"}],"pointValues":[{"id":"valid","contract":"io.github.tangerg.flame/contracts/point.json","value":{"title":"Evidence"}},{"id":"invalid","contract":"io.github.tangerg.flame/contracts/point.json","value":{"title":1}}]`),
	}
	release, err := publishPackage(t.Context(), testReleases(t), writePackage(t, files))
	if err != nil {
		t.Fatal(err)
	}
	declaration := release.Declaration()
	if len(declaration.Points) != 1 || declaration.Points[0].Contract != contract.Identity() || len(declaration.PointValues) != 1 || declaration.PointValues[0].ID != "valid" || len(declaration.Diagnostics) != 1 {
		t.Fatalf("point contract/value admission: %+v", declaration)
	}
	files["io.github.tangerg.flame/declaration.json"] = flameDeclaration(`"points":[{"id":"escaped","contract":"../point.json"}]`)
	release, err = publishPackage(t.Context(), testReleases(t), writePackage(t, files))
	if err != nil || len(release.Declaration().Points) != 0 || len(release.Declaration().Diagnostics) != 1 {
		t.Fatalf("point contract escaped its namespace: %+v, %v", release.Declaration(), err)
	}
}
