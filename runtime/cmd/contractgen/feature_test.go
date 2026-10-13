package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/jsonschema"
)

func TestFeatureCapabilityJSONPreservesIntegerSchemaLexemes(t *testing.T) {
	method, err := pluginsdk.NewMethod[struct{}, uint64](pluginsdk.MethodConfig{Name: "acme/count", Kind: capability.Query, Target: "installation", Description: "Read the original count."})
	if err != nil {
		t.Fatal(err)
	}
	data, err := featureDescriptor(method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := buildFeature(data)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "export const CAPABILITY_JSON = "
	_, encoded, found := strings.Cut(artifacts.typescript, prefix)
	if !found {
		t.Fatal("canonical capability source omitted")
	}
	encoded, _, _ = strings.Cut(encoded, ";\n")
	original, err := strconv.Unquote(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(original, "18446744073709551615") {
		t.Fatal("uint64 schema bound lost its exact lexeme")
	}
	if original != string(artifacts.capability.Identity().JSON()) {
		t.Fatal("generated capability differs from its canonical owner encoding")
	}
	contract, err := capability.Parse([]byte(original))
	if err != nil || contract.Identity() != artifacts.capability.Identity() {
		t.Fatalf("generated capability source changed identity: %v", err)
	}
}

type featureStatus string

func (featureStatus) JSONSchemaEnum() []string { return []string{"open", "resolved"} }

type featureInput struct {
	Status   featureStatus `json:"status"`
	Revision int64         `json:"revision"`
}
type featureOutput struct {
	Accepted bool `json:"accepted"`
}

type featurePage[T any] struct {
	Rows []T `json:"rows"`
}

func TestFeatureGenerationPreservesGenericOwnerReferences(t *testing.T) {
	method, err := pluginsdk.NewMethod[struct{}, featurePage[featureOutput]](pluginsdk.MethodConfig{Name: "acme/read", Kind: capability.Query, Target: "installation", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	data, err := featureDescriptor(method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildFeature(data); err != nil {
		t.Fatalf("generic owner references cannot be generated: %v", err)
	}
}

func TestFeatureGenerationReusesTheAdmittedSchemas(t *testing.T) {
	method, err := pluginsdk.NewMethod[featureInput, featureOutput](pluginsdk.MethodConfig{Name: "acme/reviews.update", Kind: capability.Mutation, Target: "installation", Description: "Update review."})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := method.Descriptor()
	data, err := featureDescriptor(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := buildFeature(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(generated.methods) != 1 || generated.methods[0].Name != descriptor.Name || !strings.Contains(generated.typescript, `"open" | "resolved"`) || !strings.Contains(generated.typescript, "revision: number;") || !strings.Contains(generated.typescript, "accepted: boolean;") {
		t.Fatalf("generated surface lost contract: %+v\n%s", generated.methods, generated.typescript)
	}
	contract, err := capability.Parse(data)
	if err != nil || generated.capability.Identity() != contract.Identity() || !strings.Contains(generated.typescript, contract.Identity().Digest()) {
		t.Fatalf("generator changed canonical capability identity: %v", err)
	}
	original, err := jsonschema.Parse(descriptor.Input)
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimPrefix(generated.methods[0].Input, "schema.json")
	bundle, err := json.Marshal(struct {
		Schema string                    `json:"$schema"`
		Ref    string                    `json:"$ref"`
		Defs   map[string]jsontext.Value `json:"$defs"`
	}{schemaDialect, root, generated.definitions})
	if err != nil {
		t.Fatal(err)
	}
	projected, err := jsonschema.Parse(bundle)
	if err != nil {
		t.Fatalf("generated Schema references are broken: %v", err)
	}
	for _, raw := range []string{`{"status":"open","revision":1}`, `{"status":"unknown","revision":1}`, `{"status":"open"}`, `{"status":"open","revision":null}`, `{"status":"open","revision":1,"extra":true}`} {
		if (original.Validate([]byte(raw)) == nil) != (projected.Validate([]byte(raw)) == nil) {
			t.Fatalf("generation changed admission for %s", raw)
		}
	}
}

func TestFeatureGenerationRefusesDuplicatesAndExternalSchemas(t *testing.T) {
	method, err := pluginsdk.NewMethod[struct{}, string](pluginsdk.MethodConfig{Name: "acme/read", Kind: capability.Query, Target: "installation", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := method.Descriptor()
	for _, descriptors := range [][]capability.MethodDescriptor{{descriptor, descriptor}, {{Name: "bad", Kind: capability.Query, Target: "installation", Description: "Read.", Input: []byte(`{"$ref":"https://example.invalid/schema"}`), Output: descriptor.Output}}} {
		data, err := featureDescriptor(descriptors...)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := buildFeature(data); err == nil {
			t.Fatalf("invalid feature contract admitted: %s", data)
		}
	}
	if featureTypeName("a/b") == featureTypeName("a_2fb") {
		t.Fatal("distinct semantic names share a generated identifier")
	}
}

func TestFeatureTypesUseTheExistingEmitterForScalarAndNullableFrames(t *testing.T) {
	for _, raw := range []string{`{"type":"string"}`, `{"oneOf":[{"type":"string"},{"type":"null"}]}`, `{"type":"boolean"}`, `{"type":"object","additionalProperties":{"type":"string"}}`} {
		node, err := featureNode([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		emitter := &tsEmitter{tsTypes: tsTypes{set: &schemaSet{defs: map[string]*schema{"Value": node}}}}
		emitter.define("Value", node)
		if strings.Contains(emitter.out.String(), "interface") {
			t.Fatalf("scalar/nullable frame was emitted as an object: %s", emitter.out.String())
		}
	}
}

func TestFeatureNullableObjectPreservesRequiredProperties(t *testing.T) {
	node, err := featureNode([]byte(`{"type":["object","null"],"properties":{"revision":{"type":"integer"}},"required":["revision"]}`))
	if err != nil {
		t.Fatal(err)
	}
	emitter := tsTypes{set: &schemaSet{defs: make(map[string]*schema)}}
	value := emitter.typeOf(node)
	if !strings.Contains(value, "revision: number") || !strings.Contains(value, "null") {
		t.Fatalf("nullable object lost its shape: %s", value)
	}
}

func TestFeatureGenerationCannotOverwriteDescriptorSource(t *testing.T) {
	for _, name := range []string{"methods.generated.json", "capability.generated.json"} {
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			source := filepath.Join(directory, name)
			data := []byte(`{"authored":"retained"}`)
			if err := os.WriteFile(source, data, 0600); err != nil {
				t.Fatal(err)
			}
			if err := runFeature(source, directory, ""); err == nil {
				t.Fatal("generator overwrote its descriptor input")
			}
			retained, err := os.ReadFile(source)
			if err != nil || string(retained) != string(data) {
				t.Fatalf("source changed: %s, %v", retained, err)
			}
		})
	}
}

func TestFeatureGenerationRefusesFormerBareMethodArray(t *testing.T) {
	if _, err := buildFeature([]byte(`[]`)); err == nil {
		t.Fatal("former generator input was accepted")
	}
}

func featureDescriptor(methods ...capability.MethodDescriptor) ([]byte, error) {
	return json.Marshal(capability.Descriptor{Protocol: capability.Protocol, Reference: "acme/reviews", Composition: capability.Collection, Scope: "installation", Methods: methods})
}

func TestFeatureGoProjectionPreservesTypedFramesAndIntegerBounds(t *testing.T) {
	type Input struct {
		Limit   *int  `json:"Limit,omitzero"`
		Enabled *bool `json:"Enabled,omitzero"`
	}
	type Output struct {
		Revision uint64
		At       time.Time
		Labels   map[string]string
	}
	method, err := pluginsdk.NewMethod[Input, Output](pluginsdk.MethodConfig{Name: "acme/read.v1", Kind: capability.Query, Target: "installation", Description: "Read a typed frame."})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := featureDescriptor(method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := buildFeature(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := buildFeatureGo(artifacts, featureGoOutput{Package: "consumer", Prefix: "Example"})
	if err != nil {
		t.Fatal(err)
	}
	source := string(generated)
	for _, required := range []string{"Revision uint64", "At", "time.Time", "map[string]string", "*bool", "*int64", "ExampleReadV1Method", "18446744073709551615"} {
		if !strings.Contains(source, required) {
			t.Fatalf("Go projection lost %s", required)
		}
	}
	if _, err := parser.ParseFile(token.NewFileSet(), "generated.go", generated, parser.AllErrors); err != nil {
		t.Fatal(err)
	}
	scoped, err := buildFeatureGo(artifacts, featureGoOutput{Package: "consumer"})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"const CapabilityJSON", "const ReadV1Method", "type ReadV1Input"} {
		if !strings.Contains(string(scoped), required) {
			t.Fatalf("package-scoped projection lost %s", required)
		}
	}
	for _, output := range []featureGoOutput{{Package: "consumer; bad", Prefix: "Example"}, {Package: "consumer", Prefix: "lowercase"}} {
		if _, err := buildFeatureGo(artifacts, output); err == nil {
			t.Fatal("invalid Go output configuration accepted")
		}
	}
}
