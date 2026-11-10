package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"go/ast"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/portable"
	"github.com/Tangerg/frame/portable/contractgen"
	"github.com/Tangerg/frame/value"
)

type ProjectionKind string

func (ProjectionKind) JSONSchemaEnum() []string { return []string{"project", "user"} }

type ProjectionInput struct {
	Kind ProjectionKind `json:"kind"`
}

type ProjectionRecord struct{ wire projectionRecordWire }

type projectionRecordWire struct {
	Name string            `json:"name"`
	Next *ProjectionRecord `json:"next,omitzero"`
}

func (ProjectionRecord) JSONSchemaModel() any           { return projectionRecordWire{} }
func (r ProjectionRecord) MarshalJSON() ([]byte, error) { return json.Marshal(r.wire) }
func (r *ProjectionRecord) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &r.wire, json.RejectUnknownMembers(true))
}

func TestFeatureNamesGenerateReadableConsumersWithoutChangingTheContract(t *testing.T) {
	method, err := pluginmcp.NewMethod[ProjectionInput, ProjectionRecord](portable.MethodConfig{
		Name: "fixture/read.v1", Kind: portable.Query, Target: "installation", Description: "Read the original typed record.",
	})
	if err != nil {
		t.Fatal(err)
	}
	capability, err := contract.NewCapability(contract.CapabilityConfig{Reference: "fixture", Composition: contract.Selection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	input := capability.Service().Descriptor()
	originalInput, originalOutput := bytes.Clone(input.Methods[0].Input), bytes.Clone(input.Methods[0].Output)
	before, err := value.EncodeJSON(input)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectFeatureNames(input)
	if err != nil {
		t.Fatal(err)
	}
	after, err := value.EncodeJSON(input)
	if err != nil || before.String() != after.String() || !bytes.Equal(originalInput, input.Methods[0].Input) || !bytes.Equal(originalOutput, input.Methods[0].Output) {
		t.Fatalf("consumer names changed the caller's descriptor: %v", err)
	}
	assertSameSchemaAdmission(t, input.Methods[0].Input, projected.Methods[0].Input,
		`{"kind":"project"}`, `{"kind":"other"}`, `{"kind":"user","extra":true}`, `{}`)
	assertSameSchemaAdmission(t, input.Methods[0].Output, projected.Methods[0].Output,
		`{"name":"root","next":{"name":"child"}}`, `{"name":"leaf"}`, `{"name":1}`, `{"name":"root","next":{}}`)

	directory := t.TempDir()
	goFile := filepath.Join(directory, "types.go")
	if err := generateFeature(capability, filepath.Join(directory, "schema"), directory, contractgen.GoOutput{Path: goFile, Package: "consumer"}); err != nil {
		t.Fatal(err)
	}
	generated, err := parser.ParseFile(token.NewFileSet(), goFile, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	names := make(map[string]bool)
	var embedded string
	ast.Inspect(generated, func(node ast.Node) bool {
		switch declaration := node.(type) {
		case *ast.TypeSpec:
			names[declaration.Name.Name] = true
		case *ast.ValueSpec:
			if len(declaration.Names) == 1 && declaration.Names[0].Name == "CapabilityJSON" {
				literal, ok := declaration.Values[0].(*ast.BasicLit)
				if !ok {
					t.Fatal("generated contract identity is not a string constant")
				}
				embedded, err = strconv.Unquote(literal.Value)
			}
		}
		return true
	})
	if err != nil || !names["ProjectionInput"] || !names["ProjectionInputKind"] || !names["ProjectionRecord"] || names["ProjectionRecordWire"] {
		t.Fatalf("Go names lost the original type or private codec owner: %v, %v", names, err)
	}
	embeddedCapability, err := contract.ParseCapability([]byte(embedded))
	if err != nil || embeddedCapability.Identity() != capability.Identity() {
		t.Fatalf("Go projection changed the callable identity: %v", err)
	}
	ts, err := os.ReadFile(filepath.Join(directory, "methods.generated.ts"))
	if err != nil || !bytes.Contains(ts, []byte(contractgen.TypeScriptStringLiteral(capability.Identity().String()))) {
		t.Fatalf("TypeScript projection changed the callable identity: %v", err)
	}
	declared, err := os.ReadFile(filepath.Join(directory, "schema", "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	published, err := contract.ParseCapability(declared)
	if err != nil || published.Identity() != capability.Identity() {
		t.Fatalf("published descriptor changed during projection: %v", err)
	}
	consumer := filepath.Join(directory, "consumer_test.go")
	if err := os.WriteFile(consumer, []byte(`package consumer
var _ = ProjectionInput{Kind: ProjectionInputKindProject}
var _ = ProjectionRecord{Name: "root", Next: &ProjectionRecord{Name: "child"}}
`), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "go", "test", "-mod=readonly", goFile, consumer)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated Go consumer: %v\n%s", err, output)
	}
}

func assertSameSchemaAdmission(t *testing.T, before, after jsontext.Value, samples ...string) {
	t.Helper()
	original, err := portable.AdmitSchema(before)
	if err != nil {
		t.Fatal(err)
	}
	projected, err := portable.AdmitSchema(after)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		if (original.Validate([]byte(sample)) == nil) != (projected.Validate([]byte(sample)) == nil) {
			t.Fatalf("definition labels changed admission for %s", sample)
		}
	}
}

func TestFeatureNamesKeepDistinctPackagesPrivateModelsAndOrdinaryDefinitions(t *testing.T) {
	first := scopeDefinitionKey("example.com/alpha/model", "Record")
	second := scopeDefinitionKey("example.com/beta/model", "Record")
	generic := scopeDefinitionKey("example.com/model.v2", "wire[example.com/model.Record]")
	plain := "model.Record"
	definitions := map[string]jsontext.Value{
		first: []byte(`{"type":"string"}`), second: []byte(`{"type":"integer"}`),
		generic: []byte(`{"type":"boolean"}`), plain: []byte(`{"type":"null"}`),
	}
	names := readableDefinitionNames(definitions)
	if names[first] != "alpha/model.Record" || names[second] != "beta/model.Record" || names[generic] != "model_v2.wire[example.com/model.Record]" || names[plain] != "" {
		t.Fatalf("qualified definition labels = %v", names)
	}
	for _, ordinary := range []string{"Item", "model.Item", "go_broken", "go_.SXRlbQ", "go_cGFjaw.SXRlbQ.extra"} {
		if _, decoded := scopeDefinitionName(ordinary); decoded {
			t.Fatalf("ordinary definition %q was treated as a Scope identity", ordinary)
		}
	}
}

func TestFeatureNamesDoNotOverwriteDefinitionsWhenPackageLabelsCollide(t *testing.T) {
	first := scopeDefinitionKey("example.com/model.v2", "Record")
	second := scopeDefinitionKey("example_com/model_v2", "Record")
	ordinary := "example_com/model_v2.Record"
	definitions := map[string]jsontext.Value{
		first: []byte(`{"type":"string"}`), second: []byte(`{"type":"integer"}`), ordinary: []byte(`{"type":"null"}`),
	}
	names := readableDefinitionNames(definitions)
	if len(names) != 2 || names[first] == names[second] || names[first] == ordinary || names[second] == ordinary {
		t.Fatalf("package labels overwrote a distinct definition: %v", names)
	}
	for range 20 {
		repeated := readableDefinitionNames(definitions)
		if repeated[first] != names[first] || repeated[second] != names[second] {
			t.Fatalf("map iteration changed projected names: %v, %v", names, repeated)
		}
	}
}

func TestFeatureNamesPreservePointersExactNumbersAndLiteralSchemaMembers(t *testing.T) {
	key := scopeDefinitionKey("example.com/model", "wire[example.com/model.Record]")
	pointer := jsontext.Pointer("").AppendToken(portable.KeywordDefs).AppendToken(key).AppendToken(portable.KeywordProperties).AppendToken("a/b~c")
	reference := (&url.URL{Fragment: string(pointer)}).String()
	literal := map[string]any{"$ref": reference, "$defs": map[string]any{key: true}}
	raw, err := json.Marshal(map[string]any{
		"$defs": map[string]any{key: map[string]any{"type": "object", "properties": map[string]any{"a/b~c": jsontext.Value(`{"type":"integer","minimum":9007199254740993}`)}}},
		"type":  "object", "properties": map[string]any{
			"value": map[string]any{"$ref": reference}, "$defs": map[string]any{"const": literal}, "$ref": map[string]any{"default": literal},
		},
		"default": literal, "examples": []any{literal},
	})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := projectSchemaNames(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertSameSchemaAdmission(t, raw, projected, `{"value":9007199254740993}`, `{"value":9007199254740992}`, `{"value":"bad"}`)
	var original, changed map[string]jsontext.Value
	if err := json.Unmarshal(raw, &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(projected, &changed); err != nil {
		t.Fatal(err)
	}
	for _, keyword := range []string{"default", "examples"} {
		before, _ := value.ParseJSON(original[keyword])
		after, _ := value.ParseJSON(changed[keyword])
		if before != after {
			t.Fatalf("%s application data was rewritten", keyword)
		}
	}
	var properties map[string]jsontext.Value
	if err := json.Unmarshal(changed[portable.KeywordProperties], &properties); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"$defs", "$ref"} {
		if !bytes.Contains(properties[name], []byte(key)) {
			t.Fatalf("literal schema-shaped data in property %s was rewritten", name)
		}
	}
	if !bytes.Contains(projected, []byte("9007199254740993")) || !strings.Contains(string(properties["value"]), "a~1b~0c") {
		t.Fatalf("numeric precision or reference suffix was lost: %s", projected)
	}
}

func scopeDefinitionKey(packagePath, name string) string {
	return "go_" + base64.RawURLEncoding.EncodeToString([]byte(packagePath)) + "." + base64.RawURLEncoding.EncodeToString([]byte(name))
}
