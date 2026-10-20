package main

import (
	json "encoding/json/v2"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/plugins/guardrails/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestPolicyReceiptsAndModelInputSharePrivateFactsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "guardrails.db")
	secret := "protected-fixture-value"
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		func() {
			store, err := openStore(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer store.db.Close()
			server, err := newServer(store)
			if err != nil {
				t.Fatal(err)
			}
			client := sdk.NewClient(&sdk.Implementation{Name: "guardrails-test", Version: "1"}, nil)
			if err := pluginsdk.RegisterClient(client); err != nil {
				t.Fatal(err)
			}
			a, b := sdk.NewInMemoryTransports()
			connection, err := server.Connect(t.Context(), a, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer connection.Close()
			session, err := client.Connect(t.Context(), b, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			policyExport, err := pluginsdk.ResolveCapability(t.Context(), session, "policy", contracts.Capability)
			if err != nil {
				t.Fatal(err)
			}
			replace, err := contracts.Replace.Bind(policyExport)
			if err != nil {
				t.Fatal(err)
			}
			get, err := contracts.Get.Bind(policyExport)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "edit-policy", Origin: capability.Human, Target: capability.Target{Scope: "installation", Reference: "guardrails"}})
			if err != nil {
				t.Fatal(err)
			}
			input := rpc.ReplaceInput{ExpectedRevision: 1, Phrases: []string{secret}}
			summary, err := replace.CallCommand(ctx, "edit-policy", input)
			if err != nil || summary.Revision != 2 || summary.ProtectedPhraseCount != 1 {
				t.Fatalf("policy command/replay: %+v %v", summary, err)
			}
			input.ExpectedRevision = 2
			if _, err := replace.CallCommand(ctx, "edit-policy", input); refusalCode(err) != "command_conflict" {
				t.Fatalf("changed command reused identity: %v", err)
			}
			current, err := get.Call(ctx, struct{}{})
			if err != nil || current != summary {
				t.Fatalf("policy query: %+v %v", current, err)
			}
			encoded, err := json.Marshal(current)
			if err != nil || strings.Contains(string(encoded), secret) {
				t.Fatal("inspection disclosed a protected value")
			}
			method, _ := pluginsdk.ModelInputMethod()
			contract, _ := pluginsdk.ModelInputContract()
			exported, err := pluginsdk.ResolveCapability(t.Context(), session, "input", contract)
			if err != nil {
				t.Fatal(err)
			}
			transform, err := method.Bind(exported)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err = pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "inspect-input", Origin: capability.Model, Target: capability.Target{Scope: "call", Reference: "inspect-input"}})
			if err != nil {
				t.Fatal(err)
			}
			for _, text := range []string{"public input", secret} {
				request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart(text)))
				before := request.Clone()
				original, err := jsoninput.Encode(request)
				if err != nil {
					t.Fatal(err)
				}
				output, err := transform.Call(ctx, pluginsdk.ModelInputRequest{Selection: pluginsdk.ModelSelection{Provider: "openai", Model: "test"}, Request: *request})
				if text == secret {
					if refusalCode(err) != "protected_content" || strings.Contains(err.Error(), secret) {
						t.Fatalf("protected content allowed or disclosed: %v", err)
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					projected, err := jsoninput.Encode(output)
					if err != nil || projected != original {
						t.Fatalf("allowed wire input changed: %s %v", projected.String(), err)
					}
				}
				if !reflect.DeepEqual(request, before) {
					t.Fatal("inspection changed caller-owned input")
				}
			}
		}()
	}
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), `UPDATE policy SET snapshot='{"revision":2}'`); err != nil {
		t.Fatal(err)
	}
	store.db.Close()
	if _, err := openStore(t.Context(), path); !errors.Is(err, errStoredPolicy) {
		t.Fatalf("missing phrase list became an empty policy: %v", err)
	}
}

func refusalCode(err error) string {
	if refusal, ok := errors.AsType[*capability.Refusal](err); ok {
		return refusal.Code
	}
	return ""
}
