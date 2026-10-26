package main

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"

	reviews "github.com/Tangerg/flame/examples/plugins/reviews/backend/contract"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/jsonschema"
)

func TestGeneratedAnalyzerPointPreservesTheConsumerContract(t *testing.T) {
	owner, err := reviews.NewAnalyzerPoint()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "package", "io.github.tangerg.flame", "contracts", "analyzers.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := extension.ParsePoint(data)
	if err != nil || artifact.Identity() != owner.Identity() {
		t.Fatalf("packaged analyzer point differs from its consumer owner: %v", err)
	}
}

func TestGeneratedContractsPreserveBackendAdmission(t *testing.T) {
	contracts, err := reviews.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join("..", "..", "..", "package", "io.github.tangerg.flame", "contracts")
	capabilityData, err := os.ReadFile(filepath.Join(directory, "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := extension.ParseCapability(capabilityData)
	if err != nil || contract.Identity() != contracts.Capability.Identity() {
		t.Fatalf("generated capability differs from the backend owner: %v", err)
	}
	methodsData, err := os.ReadFile(filepath.Join(directory, "methods.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	var methods []struct {
		Name        string               `json:"name"`
		Kind        extension.MethodKind `json:"kind"`
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
	descriptors := contracts.Capability.Descriptor().Methods
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
			for _, sample := range []string{`{}`, `{"id":"example-review","expectedRevision":1,"status":"open"}`, `{"id":"example-review","expectedRevision":1,"status":"unknown"}`, `{"type":"notFound"}`, `{"type":"receiptCapacity"}`, `{"type":"unknown"}`, `{"reviews":[]}`} {
				if (original.Validate([]byte(sample)) == nil) != (generated.Validate([]byte(sample)) == nil) {
					t.Fatalf("artifact changed schema acceptance for %s: %s", descriptor.Name, sample)
				}
			}
		}
	}
}

func TestUpdateStructuralAdmissionBoundsInputBeforeBusinessDecision(t *testing.T) {
	contracts, err := reviews.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	method, found := contracts.Capability.Method(contracts.Update.Name())
	if !found {
		t.Fatal("update method is absent")
	}
	for _, scenario := range []struct {
		name            string
		input           reviews.Update
		shape, business bool
	}{
		{"ascii_boundary", reviews.Update{ID: strings.Repeat("x", 128), ExpectedRevision: 1, Status: reviews.Open}, true, true},
		{"ascii_overflow", reviews.Update{ID: strings.Repeat("x", 129), ExpectedRevision: 1, Status: reviews.Open}, false, false},
		{"unicode_inside_byte_bound", reviews.Update{ID: strings.Repeat("界", 42), ExpectedRevision: 1, Status: reviews.Open}, true, true},
		{"unicode_outside_byte_bound", reviews.Update{ID: strings.Repeat("界", 43), ExpectedRevision: 1, Status: reviews.Open}, true, false},
		{"unicode_shape_overflow", reviews.Update{ID: strings.Repeat("界", 129), ExpectedRevision: 1, Status: reviews.Open}, false, false},
		{"empty_identifier", reviews.Update{ID: "", ExpectedRevision: 1, Status: reviews.Open}, true, false},
		{"zero_revision", reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 0, Status: reviews.Open}, true, false},
		{"unknown_status", reviews.Update{ID: reviews.ExampleReviewID, ExpectedRevision: 1, Status: "unknown"}, false, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			admitted, err := jsonvalue.Encode(scenario.input)
			if err != nil {
				t.Fatal(err)
			}
			if accepted := method.ValidateInput(admitted) == nil; accepted != scenario.shape {
				t.Fatalf("structural admission = %t, want %t", accepted, scenario.shape)
			}
			if accepted := scenario.input.Validate() == nil; accepted != scenario.business {
				t.Fatalf("business admission = %t, want %t", accepted, scenario.business)
			}
		})
	}
}

func TestRetiredReceiptCapacityOutcomeIsRefused(t *testing.T) {
	contracts, err := reviews.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	method, found := contracts.Capability.Method(contracts.Update.Name())
	if !found {
		t.Fatal("update method is absent")
	}
	if err := method.ValidateOutput(jsontext.Value(`{"type":"receiptCapacity"}`)); err == nil {
		t.Fatal("retired receipt quota still belongs to the operation contract")
	}
	if err := (reviews.UpdateResult{Type: reviews.Outcome("receiptCapacity")}).Validate(); err == nil {
		t.Fatal("retired receipt quota still belongs to the business vocabulary")
	}
}

func TestLiveCapabilityMatchesGeneratedReviewsContract(t *testing.T) {
	store := openTestStore(t, t.TempDir())
	server, err := newServer(store)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "contract-client"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	backend, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "package", "io.github.tangerg.flame", "contracts", "capability.generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := extension.ParseCapability(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pluginsdk.ResolveCapability(t.Context(), session, "reviews", contract); err != nil {
		t.Fatalf("live export differs from generated capability: %v", err)
	}
}
