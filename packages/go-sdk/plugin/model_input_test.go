package plugin

import (
	"encoding/json/jsontext"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/scope/core/chat"
)

func TestModelInputTransformationPreservesRouteToolsAndOutputEnvelope(t *testing.T) {
	contract, err := ModelInputContract()
	if err != nil {
		t.Fatal(err)
	}
	method, err := ModelInputMethod()
	if err != nil {
		t.Fatal(err)
	}
	if contract.Descriptor().Composition != extension.Decoration || method.Descriptor().Kind != extension.Query {
		t.Fatal("input transformation grants model execution")
	}
	original, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original")))
	original.Options.MaxOutputTokens = new(int64(64))
	original.Tools = []chat.ToolDefinition{{Name: "echo", Description: "Echo.", InputSchema: jsontext.Value(`{"type":"object"}`)}}
	for _, tc := range []struct {
		name   string
		change func(*chat.Request)
		valid  bool
	}{
		{"messages", func(r *chat.Request) { r.Messages[0] = chat.NewUserMessage(chat.NewTextPart("transformed")) }, true},
		{"lower output", func(r *chat.Request) { r.Options.MaxOutputTokens = new(int64(32)) }, true},
		{"model", func(r *chat.Request) { r.Options.Model = "replaced" }, false},
		{"reasoning", func(r *chat.Request) { r.Options.ReasoningEffort = "high" }, false},
		{"tool identity", func(r *chat.Request) { r.Tools[0].Name = "replaced" }, false},
		{"tool schema", func(r *chat.Request) {
			r.Tools[0].InputSchema = jsontext.Value(`{"type":"object","additionalProperties":false}`)
		}, false},
		{"higher output", func(r *chat.Request) { r.Options.MaxOutputTokens = new(int64(65)) }, false},
		{"missing output", func(r *chat.Request) { r.Options.MaxOutputTokens = nil }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := original.Clone()
			tc.change(r)
			if err := ValidateModelInput(original, r); (err == nil) != tc.valid {
				t.Fatalf("transformation admission: %v", err)
			}
		})
	}
}
