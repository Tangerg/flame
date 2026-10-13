package capability_test

import (
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
)

func testMethod(name string, kind capability.MethodKind) capability.MethodDescriptor {
	return capability.MethodDescriptor{Name: name, Kind: kind, Target: "workspace", Description: "Method.", Input: []byte(`{"type":"object","properties":{"revision":{"type":"integer"}},"required":["revision"],"additionalProperties":false}`), Output: []byte(`{"type":"string"}`)}
}

func testCapability(t *testing.T, methods ...capability.MethodDescriptor) *capability.Contract {
	t.Helper()
	contract, err := capability.New(capability.Config{Reference: "acme/records", Composition: capability.Selection, Scope: "workspace"}, methods...)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func TestCapabilityOwnsAnImmutableCanonicalContract(t *testing.T) {
	read, err := capability.AdmitMethod(testMethod("acme/read", capability.Query))
	if err != nil {
		t.Fatal(err)
	}
	write, err := capability.AdmitMethod(testMethod("acme/write", capability.Mutation))
	if err != nil {
		t.Fatal(err)
	}
	methods := []capability.MethodDescriptor{write.Descriptor(), read.Descriptor()}
	contract := testCapability(t, methods...)
	methods[0].Input[0] = 'x'
	projection := contract.Descriptor()
	if projection.Methods[0].Name != read.Name() || projection.Methods[1].Input[0] == 'x' {
		t.Fatal("capability borrowed mutable author input")
	}
	projection.Methods[0].Input[0] = 'x'
	if contract.Descriptor().Methods[0].Input[0] == 'x' {
		t.Fatal("projection changed the capability owner")
	}
	prose := write.Descriptor()
	prose.Description = "Revised prose."
	if testCapability(t, read.Descriptor(), prose).Identity() != contract.Identity() {
		t.Fatal("method order or prose changed callable identity")
	}
	encoded, err := json.Marshal(contract.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := capability.Parse(encoded)
	if err != nil || parsed.Identity() != contract.Identity() {
		t.Fatalf("generated projection changed callable identity: %v", err)
	}
	for _, change := range []struct {
		name   string
		change func(*capability.Descriptor)
	}{
		{"reference", func(d *capability.Descriptor) { d.Reference = "acme/other" }},
		{"composition", func(d *capability.Descriptor) { d.Composition = capability.Collection }},
		{"scope", func(d *capability.Descriptor) { d.Scope = "installation" }},
		{"target", func(d *capability.Descriptor) { d.Methods[0].Target = "installation" }},
		{"kind", func(d *capability.Descriptor) { d.Methods[0].Kind = capability.Mutation }},
		{"required set", func(d *capability.Descriptor) { d.Methods = d.Methods[:1] }},
	} {
		t.Run(change.name, func(t *testing.T) {
			descriptor := contract.Descriptor()
			change.change(&descriptor)
			data, err := json.Marshal(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := capability.Parse(data)
			if err != nil || changed.Identity() == contract.Identity() {
				t.Fatalf("changed callable identity was lost: %v", err)
			}
		})
	}
}

func TestCapabilityRefusesInvalidDeclarationsWithoutActivation(t *testing.T) {
	method, err := capability.AdmitMethod(testMethod("acme/read", capability.Query))
	if err != nil {
		t.Fatal(err)
	}
	contract := testCapability(t, method.Descriptor())
	for _, change := range []func(*capability.Descriptor){
		func(d *capability.Descriptor) { d.Protocol = "old" },
		func(d *capability.Descriptor) { d.Reference = "has space" },
		func(d *capability.Descriptor) { d.Scope = "" },
		func(d *capability.Descriptor) { d.Composition = "unknown" },
		func(d *capability.Descriptor) { d.Methods = nil },
		func(d *capability.Descriptor) { d.Methods = append(d.Methods, d.Methods[0]) },
		func(d *capability.Descriptor) { d.Methods[0].Input = []byte(`{}`) },
		func(d *capability.Descriptor) {
			d.Methods[0].Output = []byte(`{"$ref":"https://invalid.example/schema"}`)
		},
	} {
		descriptor := contract.Descriptor()
		change(&descriptor)
		data, err := json.Marshal(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := capability.Parse(data); err == nil {
			t.Fatalf("invalid declaration admitted: %s", data)
		}
	}
	data, err := json.Marshal(contract.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"extra":true}`)...)
	if _, err := capability.Parse(data); err == nil {
		t.Fatal("unknown declaration member admitted")
	}
}
