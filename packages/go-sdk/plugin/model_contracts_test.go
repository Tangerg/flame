package plugin

import (
	"bytes"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/scope/core/chat"
)

func TestPublishedModelContractsKeepConsumerDescriptorsIndependent(t *testing.T) {
	request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name  string
		load  func() (*extension.Capability, error)
		input any
	}{
		{"observer", ModelObserverContract, *request},
		{"input", ModelInputContract, ModelInputRequest{Selection: ModelSelection{Provider: "provider", Model: "model"}, Request: *request}},
		{"context", ModelContextContract, ModelContextRequest{Provider: "provider", Model: "model", Request: *request}},
		{"tool-proposal", ToolProposalContract, chat.ToolCall{ID: "logical-call", Name: "resource", Arguments: `{}`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			input, err := jsonvalue.Encode(test.input)
			if err != nil {
				t.Fatal(err)
			}
			invalid, err := jsonvalue.Parse([]byte(`{"unexpected":true}`))
			if err != nil {
				t.Fatal(err)
			}
			for range 16 {
				t.Run("consumer", func(t *testing.T) {
					t.Parallel()
					contract, err := test.load()
					if err != nil {
						t.Fatal(err)
					}
					descriptor := contract.Descriptor()
					original := bytes.Clone(descriptor.Methods[0].Input)
					descriptor.Methods[0].Input[0] = '!'
					other, err := test.load()
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(other.Descriptor().Methods[0].Input, original) {
						t.Fatal("one consumer changed another consumer's contract")
					}
					for _, descriptor := range other.Descriptor().Methods {
						method, found := other.Method(descriptor.Name)
						if !found {
							t.Fatal("published method is missing")
						}
						if err := method.ValidateInput(input); err != nil {
							t.Fatalf("valid input: %v", err)
						}
						if err := method.ValidateInput(invalid); err == nil {
							t.Fatal("malformed input was admitted")
						}
					}
				})
			}
		})
	}
}

func BenchmarkPublishedModelContracts(b *testing.B) {
	for _, test := range []struct {
		name string
		load func() (*extension.Capability, error)
	}{
		{"observer", ModelObserverContract},
		{"input", ModelInputContract},
		{"context", ModelContextContract},
	} {
		b.Run(test.name, func(b *testing.B) {
			if _, err := test.load(); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := test.load(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
