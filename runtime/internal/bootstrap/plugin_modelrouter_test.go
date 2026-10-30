package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"github.com/Tangerg/flame/packages/go-sdk/plugin/modelprovider"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func TestModelRouterInstalledPolicyBindsNewRunWithoutRewritingSessionOrAcceptedRetry(t *testing.T) {
	var physicalCalls sync.Map
	cfg := poMCPConfig(t)
	cfg.modelProviders = routerTestResolver{modelProviderAdapters: cfg.modelProviders, calls: &physicalCalls}
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "modelrouter"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := portable.ParseCapability([]byte(testsupport.ModelrouterCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	selected, _ := model.New("openai", "gpt-5-mini")
	replace := func(revision uint64, rules string, key string) {
		t.Helper()
		requestInput := testsupport.ModelrouterReplacePolicyV1Input{ExpectedRevision: revision}
		if err := json.Unmarshal([]byte(rules), &requestInput.Rules); err != nil {
			t.Fatal(err)
		}
		input, err := jsonvalue.Encode(requestInput)
		if err != nil {
			t.Fatal(err)
		}
		request := protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "modelrouter-policy", Contract: contracts.Identity().String(), Method: testsupport.ModelrouterReplacePolicyV1Method, Input: input.String(), Target: portable.Target{Scope: extension.ScopeInstallation, Reference: installed.ID}}
		result := r.must(delivery.PluginsCall, request, key).(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatal(result.Failure)
		}
	}
	replace(1, `[{"use":"interaction","selection":{"provider":"openai","model":"gpt-5-mini","reasoningEffort":""}}]`, "route-interaction")
	session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Routed work"}, "router-session").(*protocol.Session)
	input := protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "Run with the selected model."}}}
	start := func() *protocol.StartRunResponse {
		t.Helper()
		result := r.endpoint.Invoke(t.Context(), delivery.RunsStart, input, delivery.Options{IdempotencyKey: "routed-run", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
	replace(2, `[]`, "clear-routing")
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
	modelProviderAdapters
	calls *sync.Map
}

func (r routerTestResolver) OpenChat(_ context.Context, selection model.Selection, _ modelprovider.Connection) (modeladapter.ResolvedChat, error) {
	provider := delegateRestartModel{chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
		r.calls.Store(selection, true)
		return completedTextResponse("routed"), nil
	})}
	return modeladapter.NewResolvedChat(provider, nil, nil)
}

func (r routerTestResolver) RestoreChat(ctx context.Context, selection model.Selection, _ *modeladapter.ProviderCapture) (modeladapter.ResolvedChat, error) {
	return r.OpenChat(ctx, selection, modelprovider.Connection{})
}
