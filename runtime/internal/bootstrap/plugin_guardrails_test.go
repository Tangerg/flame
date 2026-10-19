package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/plugins/guardrails/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestGuardrailsInstalledInputRefusesDispatchWithoutRewritingAuthoredHistory(t *testing.T) {
	protected := "protected-guardrails-fixture-value"
	var protectedCalls, publicCalls atomic.Int32
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		for _, message := range request.Messages {
			if strings.Contains(message.Text(), protected) {
				protectedCalls.Add(1)
			}
			if strings.Contains(message.Text(), "Public fixture input.") {
				publicCalls.Add(1)
			}
		}
		return completedTextResponse("public response"), nil
	})}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := poMCPConfig(t)
	cfg.ChatResolver = testChatResolver(client)
	r := poMCPOpen(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "guardrails"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := rpc.NewContracts()
	if err != nil {
		t.Fatal(err)
	}
	input, err := jsoninput.Encode(rpc.ReplaceInput{ExpectedRevision: 1, Phrases: []string{protected}})
	if err != nil {
		t.Fatal(err)
	}
	result := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "guardrails-policy", Contract: contracts.Capability.Identity().String(), Method: contracts.Replace.Name(), Input: input.String(), Target: capability.Target{Scope: "installation", Reference: installed.ID}}, "protect-phrase").(*protocol.CallPluginResponse)
	if result.Failure != nil || strings.Contains(result.Output, protected) {
		t.Fatalf("policy result leaked or refused: %+v", result)
	}
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		t.Fatal(err)
	}
	r.must(delivery.PluginsSetComposition, protocol.SetPluginCompositionRequest{Contract: contract.Identity().String(), Members: []protocol.PluginCompositionMember{{InstallationID: installed.ID, Contribution: "guardrails-input"}}}, "guard-input")
	for index, text := range []string{"Public fixture input.", protected} {
		key := strconv.Itoa(index)
		session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Guarded work"}, "session-"+key).(*protocol.Session)
		started := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: text}}}, delivery.Options{IdempotencyKey: "start-" + key, RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
		if started.Failure != nil {
			t.Fatal(started.Failure)
		}
		for _, err := range started.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		runID := started.Value.(*protocol.StartRunResponse).RunID
		run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: runID}, "").(*protocol.RunRef)
		want := protocol.OutcomeCompleted
		if text == protected {
			want = protocol.OutcomeFailed
		}
		if run.Outcome == nil || run.Outcome.Type != want {
			t.Fatalf("guarded outcome: %+v", run)
		}
		items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeSession, SessionID: session.ID}}, "").(*protocol.ListItemsResponse)
		authored := false
		for _, item := range items.Data {
			if item.Type == protocol.ItemTypeUserMessage && len(item.Content) == 1 && item.Content[0].Text == text {
				authored = true
			}
		}
		if !authored {
			t.Fatal("guardrails rewrote the separately owned user history")
		}
		encoded, err := json.Marshal(run)
		if err != nil || strings.Contains(string(encoded), protected) {
			t.Fatal("Run failure disclosed protected input")
		}
	}
	if protectedCalls.Load() != 0 || publicCalls.Load() == 0 {
		t.Fatalf("provider admission: protected=%d public=%d", protectedCalls.Load(), publicCalls.Load())
	}
}
