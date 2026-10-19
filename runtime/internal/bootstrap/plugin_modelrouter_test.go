package bootstrap

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/plugins/modelrouter/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/modelrouter/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
)

func TestModelRouterInstalledPolicyBindsNewRunWithoutRewritingSessionOrAcceptedRetry(t *testing.T) {
	var physicalCalls sync.Map
	cfg := poMCPConfig(t)
	cfg.ChatResolver = routerTestResolver{ChatResolver: cfg.ChatResolver, calls: &physicalCalls}
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "modelrouter"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := model.New("openai", "gpt-5-mini")
	replace := func(revision int64, rules []domain.Rule, key string) {
		t.Helper()
		input, err := jsoninput.Encode(rpc.ReplaceInput{ExpectedRevision: revision, Rules: rules})
		if err != nil {
			t.Fatal(err)
		}
		request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "modelrouter-policy", Contract: contracts.Capability.Identity().String(), Method: contracts.Replace.Name(), Input: input.String(), Target: capability.Target{Scope: "installation", Reference: installed.ID}}
		result := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
	}
	replace(1, []domain.Rule{{Use: model.Interaction, Selection: selected}}, "route-interaction")
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Routed work"}, "router-session").(*protocol.Session)
	input := protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Run with the selected model."}}}
	start := func() *protocol.StartRunResponse {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.RunsStart, input, delivery.Options{IdempotencyKey: "routed-run", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
		for _, err := range result.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		return result.Value.(*protocol.StartRunResponse)
	}
	started := start()
	replace(2, []domain.Rule{}, "clear-routing")
	if retried := start(); retried.RunID != started.RunID {
		t.Fatal("accepted retry reran mutable routing")
	}
	run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.RunID}, "").(*protocol.RunRef)
	preference := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: session.ID}, "").(*protocol.Session)
	_, called := physicalCalls.Load(selected)
	if run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted || run.Provider != selected.Provider() || run.Model != selected.Model() || !called {
		t.Fatalf("selected identity did not reach the provider and accepted Run: %+v called=%t", run, called)
	}
	if preference.Provider != session.Provider || preference.Model != session.Model {
		t.Fatal("routing changed the separately owned Session preference")
	}
}

// The provider binds identity during construction; request options may omit it.
type routerTestResolver struct {
	ChatResolver
	calls *sync.Map
}

func (r routerTestResolver) ResolveChat(_ context.Context, selection model.Selection) (modeladapter.ResolvedChat, error) {
	provider := delegateRestartModel{chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		r.calls.Store(selection, true)
		return completedTextResponse("routed"), nil
	})}
	return modeladapter.NewResolvedChat(provider, nil)
}
