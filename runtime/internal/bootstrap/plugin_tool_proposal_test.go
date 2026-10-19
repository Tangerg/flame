package bootstrap

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func buildCallContributor(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "contributor")
	command := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./testdata/call-contributor")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build public call contributor: %s, %v", output, err)
	}
	return binary
}

func installCallContributor(t *testing.T, r *poMCPRuntime, binary, mode, trace string, contract *capability.Contract) *protocol.PluginInstallation {
	t.Helper()
	declaration, err := json.Marshal(map[string]any{"protocol": capability.Protocol, "contributions": []any{map[string]any{"id": "contributor", "server": "contributor", "export": "contributor", "contract": "io.github.tangerg.flame/contracts/contributor.json"}}})
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := json.Marshal(map[string]any{"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json", "mcpServers": map[string]any{"contributor": map[string]any{"type": "stdio", "command": "./contributor", "args": []string{mode, trace}}}})
	if err != nil {
		t.Fatal(err)
	}
	source := poMCPPackage(t, map[string]string{
		"plugin.json": `{"$schema":"https://agent-plugins.org/schemas/1.0.0/plugin.schema.json","name":"example.call-contributor","extensions":{"io.github.tangerg.flame":{"declaration":"io.github.tangerg.flame/declaration.json"}}}`,
		"mcp.json":    string(mcp), "io.github.tangerg.flame/declaration.json": string(declaration),
		"io.github.tangerg.flame/contracts/contributor.json": contract.Identity().String(),
	})
	content, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "contributor"), content, 0700); err != nil {
		t.Fatal(err)
	}
	installed := r.must(delivery.PluginsInstall, protocol.InstallPluginRequest{Source: source}, "install-"+mode).(*protocol.PluginInstallation)
	installed = r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "enable-"+mode).(*protocol.PluginInstallation)
	if len(installed.Selected.Diagnostics) != 0 || len(installed.Selected.Servers) != 1 || len(installed.Selected.Contributions) != 1 {
		t.Fatalf("public contributor package was not admitted: %+v", installed.Selected)
	}
	waitCallContributor(t, r, installed.ID)
	return installed
}

func waitCallContributor(t *testing.T, r *poMCPRuntime, installation string) {
	t.Helper()
	waitCallContributorState(t, r, installation, protocol.MCPServerConnected)
}

func waitCallContributorState(t *testing.T, r *poMCPRuntime, installation string, state protocol.MCPServerStateType) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		page := r.must(delivery.MCPServersList, struct{}{}, "").(*protocol.Page[protocol.MCPServer])
		for _, server := range page.Data {
			if server.ID.Origin.InstallationID == installation && server.Status.Type == state {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("call contributor did not reach %s: %+v", state, page.Data)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestInstalledToolProposalIsApprovedExecutedAndRetainedWithoutTransformReplay(t *testing.T) {
	binary := buildCallContributor(t)
	for _, mode := range []string{"proposal", "invalid-proposal", "withdrawn-proposal", "failed-proposal"} {
		t.Run(mode, func(t *testing.T) {
			var transformations atomic.Int32
			var sourceAvailable atomic.Bool
			sourceAvailable.Store(true)
			trace := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodGet {
					if !sourceAvailable.Load() {
						w.WriteHeader(http.StatusServiceUnavailable)
					}
					return
				}
				transformations.Add(1)
				w.WriteHeader(http.StatusNoContent)
			}))
			defer trace.Close()
			cfg := poMCPConfig(t)
			model := delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
				for _, message := range request.Messages {
					for _, part := range message.Parts {
						if part.ToolResult != nil && part.ToolResult.Name == "reviews_update_review" {
							return completedTextResponse("observed"), nil
						}
					}
				}
				if len(request.Tools) == 0 {
					return completedTextResponse("auxiliary"), nil
				}
				call := chat.ToolCall{ID: "provider-proposal", Name: "search_tools", Arguments: `{"query":"select:reviews_update_review"}`}
				for _, definition := range request.Tools {
					if definition.Name == "reviews_update_review" {
						call.Name, call.Arguments = definition.Name, `{"id":"example-review","expectedRevision":1,"status":"open"}`
					}
				}
				message := chat.NewAssistantMessage(chat.NewToolCallPart(call))
				return chat.NewResponse(&chat.Output{Message: &message, FinishReason: chat.FinishReasonToolCalls}, nil)
			})}
			client, err := chatclient.New(model, chatclient.Config{})
			if err != nil {
				t.Fatal(err)
			}
			cfg.ChatResolver = testChatResolver(client)
			r := poMCPOpen(t, cfg)
			defer func() { r.close() }()
			reviews := poMCPActivate(r, reviewExamplePackage(t))
			server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: reviews.ID}, Name: "reviews"}
			r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
			contract, err := pluginsdk.ToolProposalContract()
			if err != nil {
				t.Fatal(err)
			}
			contributorMode := mode
			if mode == "withdrawn-proposal" {
				contributorMode = "proposal"
			} else if mode == "failed-proposal" {
				contributorMode = "gated-proposal"
			}
			contributor := installCallContributor(t, r, binary, contributorMode, trace.URL, contract)
			r.must(delivery.PluginsSetComposition, protocol.SetPluginCompositionRequest{Contract: contract.Identity().String(), Members: []protocol.PluginCompositionMember{{InstallationID: contributor.ID, Contribution: "contributor"}}}, "select-proposals")
			session := r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "public proposal"}, "session").(*protocol.Session)
			meta := protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion, ClientCapabilities: &protocol.ClientCapabilities{InterruptTypes: []protocol.InterruptType{protocol.InterruptApproval}}}
			result := r.endpoint.Invoke(t.Context(), delivery.RunsStart, protocol.StartRunRequest{SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "update review"}}}, delivery.Options{IdempotencyKey: "start", RequestMeta: meta})
			if result.Failure != nil {
				t.Fatal(result.Failure)
			}
			started := result.Value.(*protocol.StartRunResponse)
			drainRun := func(result delivery.Result) {
				t.Helper()
				for _, err := range result.Events {
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			drainRun(result)
			interrupts := r.endpoint.Invoke(t.Context(), delivery.InterruptsList, protocol.ListInterruptsRequest{RootRunID: started.RunID}, delivery.Options{RequestMeta: meta})
			if interrupts.Failure != nil {
				t.Fatal(interrupts.Failure)
			}
			pending := interrupts.Value.(*protocol.Page[protocol.PendingInterruptSet])
			var approvedArguments map[string]any
			if mode != "invalid-proposal" {
				encoded, err := json.Marshal(pending)
				if err != nil || len(pending.Data) != 1 || len(pending.Data[0].Interrupts) != 1 {
					t.Fatalf("approval does not contain the final installed proposal: %s, %v", encoded, err)
				}
				payload := pending.Data[0].Interrupts[0].Payload
				if payload == nil || payload.Tool == nil || payload.Tool.Name != "reviews_update_review" || payload.Tool.Arguments["status"] != "resolved" || payload.Tool.Arguments["id"] != "example-review" {
					t.Fatalf("approval retained original or unrelated arguments: %s", encoded)
				}
				approvedArguments = payload.Tool.Arguments
				if mode == "withdrawn-proposal" {
					r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: contributor.ID, Enabled: false}, "withdraw-captured-contributor")
				}
				if mode == "failed-proposal" {
					sourceAvailable.Store(false)
				}
				r.close()
				r = poMCPOpen(t, cfg)
				r.poMCPTools(server, func(tools []protocol.MCPTool) bool { return len(tools) == 3 })
				if mode == "failed-proposal" {
					waitCallContributorState(t, r, contributor.ID, protocol.MCPServerFailed)
					if err := r.host.application.recoverUncertainExecutions(t.Context()); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "withdrawn-proposal" || mode == "failed-proposal" {
					lost := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: started.RunID}, "").(*protocol.RunRef)
					if lost.Status != protocol.RunStatusFinished || lost.Outcome == nil || lost.Outcome.Type != protocol.OutcomeLost {
						t.Fatalf("withdrawn captured contributor blocked or silently changed waiting recovery: %+v", lost)
					}
				} else {
					waitCallContributor(t, r, contributor.ID)
					result := r.endpoint.Invoke(t.Context(), delivery.RunsResume, protocol.ResumeRunRequest{RunID: started.RunID, Responses: []protocol.InterruptResponse{{ItemID: pending.Data[0].Interrupts[0].ItemID, Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseApproval, Decision: protocol.ApprovalApprove}}}}, delivery.Options{IdempotencyKey: "approve", RequestMeta: meta})
					if result.Failure != nil {
						t.Fatalf("resume captured proposal: %v; cause: %v", result.Failure, errors.Unwrap(result.Failure))
					}
					drainRun(result)
				}
			} else if len(pending.Data) != 0 {
				t.Fatalf("invalid transformed arguments reached approval: %+v", pending.Data)
			}
			items := r.must(delivery.ItemsList, protocol.ListItemsRequest{Scope: protocol.ItemListScope{Type: protocol.ItemScopeRun, RunID: started.RunID}}, "").(*protocol.ListItemsResponse)
			found := false
			for _, item := range items.Data {
				if item.Tool == nil || item.Tool.Name != "reviews_update_review" {
					continue
				}
				found = true
				if mode == "proposal" && !reflect.DeepEqual(item.Tool.Arguments, approvedArguments) {
					t.Fatalf("executed/history arguments differ from approval: approved=%+v executed=%+v", approvedArguments, item.Tool.Arguments)
				}
				encoded, err := json.Marshal(item.Tool)
				if err != nil || (mode == "proposal" && (item.Status != protocol.ItemStatusCompleted || !strings.Contains(string(encoded), `resolved`) || !strings.Contains(string(encoded), `updated`))) || (mode != "proposal" && item.Status == protocol.ItemStatusCompleted) {
					t.Fatalf("executed/history outcome differs from final admission: %+v, %s, %v", item, encoded, err)
				}
			}
			if !found || transformations.Load() != 1 {
				t.Fatalf("proposal missing or replayed across approval/restart: found=%v transformations=%d", found, transformations.Load())
			}
			read := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: reviews.ID, Digest: reviews.Selected.Digest, Contribution: "review-operations", Contract: reviews.Selected.Contributions[0].Contract, Method: "io.github.tangerg.flame.examples/reviews.list", Input: `{}`, Target: capability.Target{Scope: "installation", Reference: reviews.ID}}, "").(*protocol.CallPluginResponse)
			var current struct {
				Reviews []struct {
					Revision int64  `json:"revision"`
					Status   string `json:"status"`
				} `json:"reviews"`
			}
			if read.Failure != nil {
				t.Fatal(read.Failure)
			}
			if err := json.Unmarshal([]byte(read.Output), &current); err != nil || len(current.Reviews) != 1 {
				t.Fatalf("private owner read failed: %s, %v", read.Output, err)
			}
			expectedRevision, expectedStatus := int64(2), "resolved"
			if mode != "proposal" {
				expectedRevision, expectedStatus = 1, "open"
			}
			if current.Reviews[0].Revision != expectedRevision || current.Reviews[0].Status != expectedStatus {
				t.Fatalf("private owner differs from final admission: %+v", current.Reviews)
			}
		})
	}
}
