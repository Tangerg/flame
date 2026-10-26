package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"sync/atomic"
	"testing"

	contractsdk "github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func testCapability(t *testing.T, methods ...contractsdk.MethodDescriptor) *contractsdk.Contract {
	t.Helper()
	capability, err := contractsdk.New(contractsdk.Config{Reference: "acme/records", Composition: contractsdk.Selection, Scope: "workspace"}, methods...)
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func TestCapabilityPublicationAndDispatchCheckTheWholeExport(t *testing.T) {
	read, err := NewMethod[struct{}, string](MethodConfig{Name: "acme/read", Kind: contractsdk.Query, Target: "workspace", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	write, err := NewMethod[string, string](MethodConfig{Name: "acme/write", Kind: contractsdk.Mutation, Target: "workspace", Description: "Write."})
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
	exports := []CapabilityExport{{Name: "records", Contract: capability.Descriptor()}, {Name: "reader", Contract: readOnly.Descriptor()}}
	if err := VerifyCapabilities(t.Context(), session, exports); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]CapabilityExport{
		exports[:1],
		{{Name: "records", Contract: readOnly.Descriptor()}, exports[1]},
		{exports[0], exports[0]},
		{{Name: "unknown", Contract: capability.Descriptor()}, exports[1]},
	} {
		if err := VerifyCapabilities(t.Context(), session, invalid); err == nil {
			t.Fatal("different declared export set was admitted")
		}
	}
	if err := backend.BindCapability("late", capability); err == nil {
		t.Fatal("published export changed")
	}
	if err := backend.BindCapability("zero", &contractsdk.Contract{}); err == nil {
		t.Fatal("unvalidated capability bound")
	}
	for _, attempt := range []struct {
		export string
		need   *contractsdk.Contract
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
		{session: session, method: member.method},
		{session: session, method: member.method, export: "missing", capability: member.capability},
		{session: session, method: member.method, export: "records", capability: readOnly.Identity().Digest()},
		{session: session, method: member.method, export: "reader", capability: readOnly.Identity().Digest()},
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
	method, err := NewMethod[string, string](MethodConfig{Name: "acme/write", Kind: contractsdk.Mutation, Target: "installation", Description: "Write."})
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
	method, err := NewMethod[struct{}, string](MethodConfig{Name: "acme/read", Kind: contractsdk.Query, Target: "installation", Description: "Read."})
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
		{"wrong required method", func(d *Description) { d.Methods[0].Kind = contractsdk.Mutation }},
		{"duplicate export", func(d *Description) { d.Capabilities = append(d.Capabilities, d.Capabilities[0]) }},
		{"missing export", func(d *Description) { d.Capabilities = nil }},
	} {
		t.Run(change.name, func(t *testing.T) {
			description := Description{Protocol: contractsdk.Protocol, Methods: []contractsdk.MethodDescriptor{method.Descriptor()}, Capabilities: []CapabilityExport{{Name: "reader", Contract: capability.Descriptor()}}}
			change.change(&description)
			server := sdk.NewServer(&sdk.Implementation{Name: "invalid-publication"}, nil)
			if err := sdk.AddReceivingCustomMethod(server, describeMethod, func(context.Context, *sdk.ServerSession, *protocolParams) (*Result[Description], error) {
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
