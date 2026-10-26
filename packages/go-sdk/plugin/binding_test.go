package plugin

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestCapturedContractRejectsChangesBeforeBusinessExecution(t *testing.T) {
	var calls atomic.Int32
	session := methodSession(t, func(backend *Backend) error {
		return testMethod[string, string](t, "owner.write", extension.Mutation).RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) (string, error) {
			calls.Add(1)
			return command.Input, nil
		})
	}, "owner.write")
	original, err := Resolve(t.Context(), session, "owner.write")
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Encode("input")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		name   string
		change func(*extension.MethodDescriptor)
	}{
		{"method", func(d *extension.MethodDescriptor) { d.Name = "another.write" }},
		{"target", func(d *extension.MethodDescriptor) { d.Target = "another-target" }},
		{"kind", func(d *extension.MethodDescriptor) { d.Kind = extension.Query }},
		{"input", func(d *extension.MethodDescriptor) { d.Input = jsontext.Value(`{"type":"string","minLength":1}`) }},
		{"output", func(d *extension.MethodDescriptor) { d.Output = jsontext.Value(`{"type":"string","minLength":1}`) }},
	} {
		t.Run(change.name, func(t *testing.T) {
			descriptor := original.Descriptor()
			change.change(&descriptor)
			binding, err := NewBinding(session, descriptor)
			if err != nil {
				t.Fatal(err)
			}
			id := "command"
			if descriptor.Kind == extension.Query {
				id = ""
			}
			if _, err := binding.Invoke(t.Context(), id, input); err == nil {
				t.Fatal("changed contract reached business execution")
			}
			if calls.Load() != 0 {
				t.Fatalf("contract rejection executed owner %d times", calls.Load())
			}
		})
	}
	descriptor := original.Descriptor()
	descriptor.Description = "revised prose"
	prose, err := NewBinding(session, descriptor)
	if err != nil || prose.Contract() != original.Contract() {
		t.Fatalf("prose changed callable contract: %v", err)
	}
	descriptor.Input[0] = 'x'
	if value, err := prose.Invoke(t.Context(), "command", input); err != nil || value != input || calls.Load() != 1 {
		t.Fatalf("captured contract changed: %v, %v, calls=%d", value, err, calls.Load())
	}
	if _, err := original.Invoke(t.Context(), "", input); err == nil || calls.Load() != 1 {
		t.Fatal("mutation ran without command identity")
	}
}

func TestOriginalBindingValidatesPrivateReceiptWithoutInvokingMutation(t *testing.T) {
	type output struct {
		Revision int `json:"revision"`
	}
	var mutations, reads atomic.Int32
	reply := jsontext.Value(`{"value":{}}`)
	method, err := NewMethod[string, output](MethodConfig{Name: "owner.update", Kind: extension.Mutation, Target: "workspace", Description: "update"})
	if err != nil {
		t.Fatal(err)
	}
	session := methodSession(t, func(backend *Backend) error {
		if err := method.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[string]) (output, error) {
			mutations.Add(1)
			return output{Revision: 2}, nil
		}); err != nil {
			return err
		}
		return testMethod[ReceiptRequest, Receipt](t, ReceiptMethod, extension.Query).RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[ReceiptRequest]) (Receipt, error) {
			reads.Add(1)
			return Receipt{Found: true, Reply: reply}, nil
		})
	}, "owner.update")
	binding, err := NewBinding(session, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsonvalue.Encode("input")
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := binding.ReadReceipt(t.Context(), "original", input); err == nil || found {
		t.Fatal("receipt missing required revision became original completion")
	}
	reply = jsontext.Value(`{"value":{"revision":2}}`)
	if value, found, err := binding.ReadReceipt(t.Context(), "original", input); err != nil || !found || value.String() != `{"revision":2}` {
		t.Fatalf("original receipt=%v, %t, %v", value, found, err)
	}
	reply = jsontext.Value(`{"failure":{"code":"revision","message":"conflict"}}`)
	if _, found, err := binding.ReadReceipt(t.Context(), "original", input); !found {
		t.Fatalf("known refusal disappeared: %v", err)
	} else if failure, ok := errors.AsType[*extension.Refusal](err); !ok || failure.Code != "revision" {
		t.Fatalf("original refusal changed: %v", err)
	}
	if mutations.Load() != 0 || reads.Load() != 3 {
		t.Fatalf("receipt lookup reentered owner: mutations=%d, reads=%d", mutations.Load(), reads.Load())
	}
}

func TestBindingRefusesUnavailableOrExternallyReferencedContract(t *testing.T) {
	session := methodSession(t, func(backend *Backend) error {
		return testMethod[struct{}, string](t, "owner.read", extension.Query).RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[struct{}]) (string, error) { return "value", nil })
	}, "owner.read")
	if _, err := Resolve(t.Context(), session, "missing"); err == nil {
		t.Fatal("missing contract resolved")
	}
	binding, err := Resolve(t.Context(), session, "owner.read")
	if err != nil {
		t.Fatal(err)
	}
	descriptor := binding.Descriptor()
	descriptor.Input = jsontext.Value(`{"$ref":"https://invalid.example/schema.json"}`)
	if _, err := NewBinding(session, descriptor); err == nil {
		t.Fatal("contract enabled external schema loading")
	}
	descriptor = binding.Descriptor()
	descriptor.Input = jsontext.Value(`{}`)
	if _, err := NewBinding(session, descriptor); err == nil {
		t.Fatal("contract admitted absent request")
	}
}
