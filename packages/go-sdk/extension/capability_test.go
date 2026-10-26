package extension_test

import (
	json "encoding/json/v2"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
)

func testMethod(name string, kind extension.MethodKind) extension.MethodDescriptor {
	return extension.MethodDescriptor{Name: name, Kind: kind, Target: "workspace", Description: "Method.", Input: []byte(`{"type":"object","properties":{"revision":{"type":"integer"}},"required":["revision"],"additionalProperties":false}`), Output: []byte(`{"type":"string"}`)}
}

func testCapability(t *testing.T, methods ...extension.MethodDescriptor) *extension.Capability {
	t.Helper()
	contract, err := extension.NewCapability(extension.CapabilityConfig{Reference: "acme/records", Composition: extension.Selection, Scope: "workspace"}, methods...)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func TestCapabilityOwnsAnImmutableCanonicalContract(t *testing.T) {
	read, err := extension.AdmitMethod(testMethod("acme/read", extension.Query))
	if err != nil {
		t.Fatal(err)
	}
	write, err := extension.AdmitMethod(testMethod("acme/write", extension.Mutation))
	if err != nil {
		t.Fatal(err)
	}
	methods := []extension.MethodDescriptor{write.Descriptor(), read.Descriptor()}
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
	parsed, err := extension.ParseCapability(encoded)
	if err != nil || parsed.Identity() != contract.Identity() {
		t.Fatalf("generated projection changed callable identity: %v", err)
	}
	for _, change := range []struct {
		name   string
		change func(*extension.CapabilityDescriptor)
	}{
		{"reference", func(d *extension.CapabilityDescriptor) { d.Reference = "acme/other" }},
		{"composition", func(d *extension.CapabilityDescriptor) { d.Composition = extension.Collection }},
		{"scope", func(d *extension.CapabilityDescriptor) { d.Scope = "installation" }},
		{"target", func(d *extension.CapabilityDescriptor) { d.Methods[0].Target = "installation" }},
		{"kind", func(d *extension.CapabilityDescriptor) { d.Methods[0].Kind = extension.Mutation }},
		{"required set", func(d *extension.CapabilityDescriptor) { d.Methods = d.Methods[:1] }},
	} {
		t.Run(change.name, func(t *testing.T) {
			descriptor := contract.Descriptor()
			change.change(&descriptor)
			data, err := json.Marshal(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := extension.ParseCapability(data)
			if err != nil || changed.Identity() == contract.Identity() {
				t.Fatalf("changed callable identity was lost: %v", err)
			}
		})
	}
}

func TestCapabilityRefusesInvalidDeclarationsWithoutActivation(t *testing.T) {
	method, err := extension.AdmitMethod(testMethod("acme/read", extension.Query))
	if err != nil {
		t.Fatal(err)
	}
	contract := testCapability(t, method.Descriptor())
	for _, change := range []func(*extension.CapabilityDescriptor){
		func(d *extension.CapabilityDescriptor) { d.Protocol = "old" },
		func(d *extension.CapabilityDescriptor) { d.Reference = "has space" },
		func(d *extension.CapabilityDescriptor) { d.Scope = "" },
		func(d *extension.CapabilityDescriptor) { d.Composition = "unknown" },
		func(d *extension.CapabilityDescriptor) { d.Methods = nil },
		func(d *extension.CapabilityDescriptor) { d.Methods = append(d.Methods, d.Methods[0]) },
		func(d *extension.CapabilityDescriptor) { d.Methods[0].Input = []byte(`{}`) },
		func(d *extension.CapabilityDescriptor) {
			d.Methods[0].Output = []byte(`{"$ref":"https://invalid.example/schema"}`)
		},
	} {
		descriptor := contract.Descriptor()
		change(&descriptor)
		data, err := json.Marshal(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := extension.ParseCapability(data); err == nil {
			t.Fatalf("invalid declaration admitted: %s", data)
		}
	}
	data, err := json.Marshal(contract.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"extra":true}`)...)
	if _, err := extension.ParseCapability(data); err == nil {
		t.Fatal("unknown declaration member admitted")
	}
}
