package reviews

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/jsonschema"
)

func TestGeneratedContractsPreserveBackendAdmission(t *testing.T) {
	contracts, err := NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join("..", "package", "io.github.tangerg.flame", "contracts")
	methodsData, err := os.ReadFile(filepath.Join(directory, "methods.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	var methods []struct {
		Name        string               `json:"name"`
		Kind        pluginsdk.MethodKind `json:"kind"`
		Target      string               `json:"target"`
		Description string               `json:"description"`
		Input       string               `json:"input"`
		Output      string               `json:"output"`
	}
	if err := json.Unmarshal(methodsData, &methods, json.RejectUnknownMembers(true)); err != nil {
		t.Fatal(err)
	}
	schemaData, err := os.ReadFile(filepath.Join(directory, "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]jsontext.Value
	if err := json.Unmarshal(schemaData, &document); err != nil {
		t.Fatal(err)
	}
	descriptors := []pluginsdk.MethodDescriptor{contracts.List.Descriptor(), contracts.Update.Descriptor()}
	if len(methods) != len(descriptors) {
		t.Fatal("generated method surface differs from its backend owner")
	}
	for index, descriptor := range descriptors {
		method := methods[index]
		if method.Name != descriptor.Name || method.Kind != descriptor.Kind || method.Target != descriptor.Target || method.Description != descriptor.Description {
			t.Fatalf("generated method changed authority: %+v", method)
		}
		for _, frame := range []struct {
			reference string
			schema    jsontext.Value
		}{{method.Input, descriptor.Input}, {method.Output, descriptor.Output}} {
			ref, local := strings.CutPrefix(frame.reference, "schema.json")
			if !local || !strings.HasPrefix(ref, "#/$defs/") {
				t.Fatalf("generated reference is not self-contained: %s", frame.reference)
			}
			document["$ref"], err = json.Marshal(ref)
			if err != nil {
				t.Fatal(err)
			}
			compiled, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			generated, err := jsonschema.Parse(compiled)
			if err != nil {
				t.Fatal(err)
			}
			original, err := jsonschema.Parse(frame.schema)
			if err != nil {
				t.Fatal(err)
			}
			for _, sample := range []string{`{}`, `{"id":"example-review","expectedRevision":1,"status":"open"}`, `{"id":"example-review","expectedRevision":1,"status":"unknown"}`, `{"type":"notFound"}`, `{"type":"unknown"}`, `{"reviews":[]}`} {
				if (original.Validate([]byte(sample)) == nil) != (generated.Validate([]byte(sample)) == nil) {
					t.Fatalf("artifact changed schema acceptance for %s: %s", descriptor.Name, sample)
				}
			}
		}
	}
}
