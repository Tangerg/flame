package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func testCapability(t *testing.T, methods ...MethodDescriptor) *Capability {
	t.Helper()
	capability, err := NewCapability(CapabilityConfig{Reference: "acme/records", Composition: Selection, Scope: "workspace"}, methods...)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func TestCapabilityOwnsAnImmutableCanonicalContract(t *testing.T) {
	read, err := NewMethod[struct{}, string](MethodConfig{Name: "acme/read", Kind: Query, Target: "workspace", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	write, err := NewMethod[string, string](MethodConfig{Name: "acme/write", Kind: Mutation, Target: "workspace", Description: "Write."})
	if err != nil {
		t.Fatal(err)
	}
	methods := []MethodDescriptor{write.Descriptor(), read.Descriptor()}
	capability := testCapability(t, methods...)
	methods[0].Input[0] = 'x'
	projection := capability.Descriptor()
	if projection.Methods[0].Name != read.Name() || projection.Methods[1].Input[0] == 'x' {
		t.Fatal("capability borrowed mutable author input")
	}
	projection.Methods[0].Input[0] = 'x'
	if capability.Descriptor().Methods[0].Input[0] == 'x' {
		t.Fatal("projection changed the capability owner")
	}
	prose := write.Descriptor()
	prose.Description = "Revised prose."
	if testCapability(t, read.Descriptor(), prose).Contract() != capability.Contract() {
		t.Fatal("method order or prose changed callable identity")
	}
	encoded, err := json.Marshal(capability.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseCapability(encoded)
	if err != nil || parsed.Contract() != capability.Contract() {
		t.Fatalf("generated projection changed callable identity: %v", err)
	}
	for _, change := range []struct {
		name   string
		change func(*CapabilityDescriptor)
	}{
		{"reference", func(d *CapabilityDescriptor) { d.Reference = "acme/other" }},
		{"composition", func(d *CapabilityDescriptor) { d.Composition = Collection }},
		{"scope", func(d *CapabilityDescriptor) { d.Scope = "installation" }},
		{"target", func(d *CapabilityDescriptor) { d.Methods[0].Target = "installation" }},
		{"kind", func(d *CapabilityDescriptor) { d.Methods[0].Kind = Mutation }},
		{"required set", func(d *CapabilityDescriptor) { d.Methods = d.Methods[:1] }},
	} {
		t.Run(change.name, func(t *testing.T) {
			descriptor := capability.Descriptor()
			change.change(&descriptor)
			data, err := json.Marshal(descriptor)
			if err != nil {
				t.Fatal(err)
			}
			changed, err := ParseCapability(data)
			if err != nil || changed.Contract() == capability.Contract() {
				t.Fatalf("changed callable identity was lost: %v", err)
			}
		})
	}
}

func TestCapabilityRefusesInvalidDeclarationsWithoutActivation(t *testing.T) {
	method, err := NewMethod[struct{}, string](MethodConfig{Name: "acme/read", Kind: Query, Target: "installation", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	capability := testCapability(t, method.Descriptor())
	for _, change := range []func(*CapabilityDescriptor){
		func(d *CapabilityDescriptor) { d.Protocol = "old" },
		func(d *CapabilityDescriptor) { d.Reference = "has space" },
		func(d *CapabilityDescriptor) { d.Scope = "" },
		func(d *CapabilityDescriptor) { d.Composition = "unknown" },
		func(d *CapabilityDescriptor) { d.Methods = nil },
		func(d *CapabilityDescriptor) { d.Methods = append(d.Methods, d.Methods[0]) },
		func(d *CapabilityDescriptor) { d.Methods[0].Input = []byte(`{}`) },
		func(d *CapabilityDescriptor) {
			d.Methods[0].Output = []byte(`{"$ref":"https://invalid.example/schema"}`)
		},
	} {
		descriptor := capability.Descriptor()
		change(&descriptor)
		data, err := json.Marshal(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ParseCapability(data); err == nil {
			t.Fatalf("invalid declaration admitted: %s", data)
		}
	}
	data, err := json.Marshal(capability.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	data = append(data[:len(data)-1], []byte(`,"extra":true}`)...)
	if _, err := ParseCapability(data); err == nil {
		t.Fatal("unknown declaration member admitted")
	}
}

func TestCapabilityPublicationAndDispatchCheckTheWholeExport(t *testing.T) {
	read, err := NewMethod[struct{}, string](MethodConfig{Name: "acme/read", Kind: Query, Target: "workspace", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	write, err := NewMethod[string, string](MethodConfig{Name: "acme/write", Kind: Mutation, Target: "workspace", Description: "Write."})
	if err != nil {
		t.Fatal(err)
	}
	capability := testCapability(t, read.Descriptor(), write.Descriptor())
	readOnly := testCapability(t, read.Descriptor())
	backend := testBackend(t, sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil))
	var reads, writes atomic.Int32
	if err := read.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (string, error) {
		reads.Add(1)
		return "read", nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("records", capability); err == nil {
		t.Fatal("incomplete capability was published")
	}
	if err := write.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) (string, error) {
		writes.Add(1)
		return command.Input, nil
	}); err != nil {
		t.Fatal(err)
	}
	wrong := write.Descriptor()
	wrong.Output = []byte(`{"type":"integer"}`)
	if err := backend.BindCapability("records", testCapability(t, read.Descriptor(), wrong)); err == nil {
		t.Fatal("capability accepted a different required schema")
	}
	if err := backend.BindCapability("records", capability); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("reader", readOnly); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("records", capability); err == nil {
		t.Fatal("duplicate export replaced a binding")
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	if err := backend.BindCapability("late", capability); err == nil {
		t.Fatal("published export changed")
	}
	if err := backend.BindCapability("zero", &Capability{}); err == nil {
		t.Fatal("unvalidated capability bound")
	}
	for _, attempt := range []struct {
		export string
		need   *Capability
	}{{"missing", capability}, {"reader", capability}} {
		if _, err := ResolveCapability(t.Context(), session, attempt.export, attempt.need); err == nil {
			t.Fatal("unavailable/mismatched export resolved")
		}
	}
	binding, err := ResolveCapability(t.Context(), session, "records", capability)
	if err != nil {
		t.Fatal(err)
	}
	member, err := binding.Method(write.Name())
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsoninput.Encode("input")
	if err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []Binding{
		{session: session, descriptor: member.descriptor, input: member.input, output: member.output, contract: member.contract},
		{session: session, descriptor: member.descriptor, input: member.input, output: member.output, contract: member.contract, export: "missing", capability: member.capability},
		{session: session, descriptor: member.descriptor, input: member.input, output: member.output, contract: member.contract, export: "records", capability: readOnly.Contract().Digest()},
		{session: session, descriptor: member.descriptor, input: member.input, output: member.output, contract: member.contract, export: "reader", capability: readOnly.Contract().Digest()},
	} {
		if _, err := attempt.Invoke(t.Context(), "original", input); err == nil {
			t.Fatal("invalid export identity reached the owner")
		}
	}
	if reads.Load() != 0 || writes.Load() != 0 {
		t.Fatal("refused binding executed business code")
	}
	restored, err := NewCapabilityBinding(session, "records", capability)
	if err != nil {
		t.Fatal(err)
	}
	method, err := write.Bind(restored)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := method.CallCommand(t.Context(), "original", "input"); err != nil || result != "input" || writes.Load() != 1 {
		t.Fatalf("captured export dispatch = %s, %v, %d", result, err, writes.Load())
	}
	if _, err := method.Call(t.Context(), "input"); err == nil || writes.Load() != 1 {
		t.Fatal("command was relabeled as a query")
	}
	if _, err := binding.Method("undeclared"); err == nil {
		t.Fatal("capability accessed a nonmember method")
	}
	reader, err := ResolveCapability(t.Context(), session, "reader", readOnly)
	if err != nil {
		t.Fatal(err)
	}
	readMethod, err := read.Bind(reader)
	if err != nil {
		t.Fatal(err)
	}
	if value, err := readMethod.Call(t.Context(), struct{}{}); err != nil || value != "read" || reads.Load() != 1 {
		t.Fatalf("second export in the same process = %s, %v", value, err)
	}
}

func TestCapabilityReceiptRecoveryKeepsTheCapturedExport(t *testing.T) {
	method, err := NewMethod[string, string](MethodConfig{Name: "acme/write", Kind: Mutation, Target: "installation", Description: "Write."})
	if err != nil {
		t.Fatal(err)
	}
	capability := testCapability(t, method.Descriptor())
	var calls, lookups atomic.Int32
	receipts := map[string][]byte{}
	session := methodSession(t, func(backend *Backend) error {
		if err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) (string, error) {
			calls.Add(1)
			receipts[command.ID], _ = json.Marshal(Result[string]{Value: &command.Input})
			return "", errors.New("observation lost after commit")
		}); err != nil {
			return err
		}
		if err := RegisterServer(backend, ReceiptMethod, nil, func(_ context.Context, _ *sdk.ServerSession, request ReceiptRequest) (Receipt, error) {
			lookups.Add(1)
			reply := receipts[request.ID]
			return Receipt{Found: len(reply) > 0, Reply: reply}, nil
		}); err != nil {
			return err
		}
		return backend.BindCapability("records", capability)
	}, method.Name())
	resolved, err := ResolveCapability(t.Context(), session, "records", capability)
	if err != nil {
		t.Fatal(err)
	}
	write, err := method.Bind(resolved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := write.CallCommand(t.Context(), "original", "input"); err == nil {
		t.Fatal("unknown observation became success")
	}
	if _, found, err := ReadReceipt[string, string](t.Context(), session, "original", method.Name(), "input"); err == nil || found || lookups.Load() != 0 {
		t.Fatal("bare receipt query bypassed the captured export")
	}
	for _, name := range []string{"missing", "records"} {
		restored, err := NewCapabilityBinding(session, name, capability)
		if err != nil {
			t.Fatal(err)
		}
		original, err := method.Bind(restored)
		if err != nil {
			t.Fatal(err)
		}
		value, found, err := original.ReadReceipt(t.Context(), "original", "input")
		if name == "missing" {
			if err == nil || found || lookups.Load() != 0 {
				t.Fatal("receipt lookup moved to a replacement export")
			}
		} else if err != nil || !found || value != "input" {
			t.Fatalf("original receipt = %s, %t, %v", value, found, err)
		}
	}
	if calls.Load() != 1 || lookups.Load() != 1 {
		t.Fatalf("receipt recovery repeated mutation: %d writes, %d lookups", calls.Load(), lookups.Load())
	}
}

func TestCapabilityDiscoveryRefusesIncompleteOrAmbiguousPublication(t *testing.T) {
	method, err := NewMethod[struct{}, string](MethodConfig{Name: "acme/read", Kind: Query, Target: "installation", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	capability := testCapability(t, method.Descriptor())
	for _, change := range []struct {
		name   string
		change func(*Description)
	}{
		{"missing required method", func(d *Description) { d.Methods = nil }},
		{"duplicate method", func(d *Description) { d.Methods = append(d.Methods, d.Methods[0]) }},
		{"wrong required method", func(d *Description) { d.Methods[0].Kind = Mutation }},
		{"duplicate export", func(d *Description) { d.Capabilities = append(d.Capabilities, d.Capabilities[0]) }},
		{"missing export", func(d *Description) { d.Capabilities = nil }},
	} {
		t.Run(change.name, func(t *testing.T) {
			description := Description{Protocol: BackendProtocol, Methods: []MethodDescriptor{method.Descriptor()}, Capabilities: []CapabilityExport{{Name: "reader", Contract: capability.Descriptor()}}}
			change.change(&description)
			server := sdk.NewServer(&sdk.Implementation{Name: "invalid-publication"}, nil)
			if err := sdk.AddReceivingCustomMethod(server, describeMethod, func(context.Context, *sdk.ServerSession, *describeParams) (*Result[Description], error) {
				return Reply(description, nil, nil)
			}); err != nil {
				t.Fatal(err)
			}
			client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
			if err := RegisterClient(client); err != nil {
				t.Fatal(err)
			}
			session := connectMethodSessions(t, server, client)
			if _, err := ResolveCapability(t.Context(), session, "reader", capability); err == nil {
				t.Fatal("unusable complete capability was returned to its consumer")
			}
		})
	}
}
