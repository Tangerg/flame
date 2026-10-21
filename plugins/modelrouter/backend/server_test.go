package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	routing "github.com/Tangerg/flame/plugins/modelrouter/backend/domain"
	"github.com/Tangerg/flame/plugins/modelrouter/backend/rpc"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/model"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestPolicyCommandsAndSelectionShareTheDurableOwnerAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := model.NewWithReasoningEffort("openai", "small", "low")
	input := rpc.ReplaceInput{ExpectedRevision: 1, Rules: []routing.Rule{{Use: model.Utility, Selection: selected}}}
	for _, reopened := range []bool{false, true} {
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
			client := sdk.NewClient(&sdk.Implementation{Name: "router-test", Version: "1"}, nil)
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
			ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "policy-edit", Origin: capability.Human, Target: capability.Target{Scope: "installation", Reference: "router"}})
			if err != nil {
				t.Fatal(err)
			}
			result, err := replace.CallCommand(ctx, "policy-edit", input)
			if err != nil || result.Revision != 2 || !reflect.DeepEqual(result.Rules, input.Rules) {
				t.Fatalf("command/replay reopened=%t: %+v %v", reopened, result, err)
			}
			if _, err := replace.CallCommand(ctx, "stale-policy", input); refusalCode(err) != "revision_conflict" {
				t.Fatalf("stale policy was accepted: %v", err)
			}
			changed := input
			changed.ExpectedRevision = 2
			if _, err := replace.CallCommand(ctx, "policy-edit", changed); refusalCode(err) != "command_conflict" {
				t.Fatalf("command identity was rebound: %v", err)
			}
			query, _ := pluginsdk.ModelSelectionMethod()
			selectionContract, _ := pluginsdk.ModelSelectionContract()
			selectionExport, err := pluginsdk.ResolveCapability(t.Context(), session, "selection", selectionContract)
			if err != nil {
				t.Fatal(err)
			}
			selectModel, err := query.Bind(selectionExport)
			if err != nil {
				t.Fatal(err)
			}
			routeContext, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "route", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "route"}})
			if err != nil {
				t.Fatal(err)
			}
			routed, err := selectModel.Call(routeContext, pluginsdk.ModelSelectionRequest{Context: model.SelectionContext{Use: model.Utility, Limits: &model.SelectionLimits{InputKind: model.TextBytes, MaxInputBytes: 4096, MaxOutputTokens: 64}}, Proposed: pluginsdk.ModelSelection{Provider: "anthropic", Model: "large"}})
			if err != nil || routed.Provider != "openai" || routed.Model != "small" || routed.ReasoningEffort != "low" {
				t.Fatalf("persisted route: %+v %v", routed, err)
			}
			current, err := get.Call(ctx, struct{}{})
			if err != nil || !reflect.DeepEqual(current, result) {
				t.Fatalf("queries changed the policy: %+v %v", current, err)
			}
		}()
	}
	store, err := openStore(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(t.Context(), `DELETE FROM policy`); err != nil {
		t.Fatal(err)
	}
	store.db.Close()
	if _, err := openStore(t.Context(), path); !errors.Is(err, errStoredPolicy) {
		t.Fatalf("missing owner fact was recreated: %v", err)
	}
}

func refusalCode(err error) string {
	if refusal, ok := errors.AsType[*capability.Refusal](err); ok {
		return refusal.Code
	}
	return ""
}
