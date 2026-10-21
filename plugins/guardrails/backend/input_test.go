package main

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"testing"

	guardrail "github.com/Tangerg/flame/plugins/guardrails/backend/domain"
	"github.com/Tangerg/scope/core/chat"
)

func TestInspectionCoversComposedTextToolContractsAndOptionsWithJSONEscaping(t *testing.T) {
	phrase := "patient@example.test"
	policy, err := guardrail.NewPolicy().Replace(1, []string{phrase, "private-\"quoted\""})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		change func(*chat.Request)
	}{
		{"context", func(r *chat.Request) {
			r.Messages = append([]chat.Message{chat.NewSystemMessage("Retrieved: " + phrase)}, r.Messages...)
		}},
		{"quoted text", func(r *chat.Request) { r.Messages[0] = chat.NewUserMessage(chat.NewTextPart("private-\"quoted\"")) }},
		{"tool description", func(r *chat.Request) {
			r.Tools = []chat.ToolDefinition{{Name: "read", Description: phrase, InputSchema: jsontext.Value(`{"type":"object"}`)}}
		}},
		{"escaped schema", func(r *chat.Request) {
			r.Tools = []chat.ToolDefinition{{Name: "read", InputSchema: jsontext.Value(`{"type":"object","description":"\u0070atient@example.test"}`)}}
		}},
		{"options", func(r *chat.Request) { r.Options.Stop = []string{phrase} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("public input")))
			tc.change(request)
			if err := request.Validate(); err != nil {
				t.Fatal(err)
			}
			if err := inspectRequest(t.Context(), policy, request); !errors.Is(err, guardrail.ErrProtectedContent) {
				t.Fatalf("protected field passed: %v", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("public input")))
	if err := inspectRequest(ctx, policy, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation became an allowed request: %v", err)
	}
}
