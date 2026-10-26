package bootstrap

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestFilewatchInstalledIngressReconcilesLostOpeningAcrossDisableAndRestart(t *testing.T) {
	cfg := poMCPConfig(t)
	var calls atomic.Int32
	var input atomic.Value
	input.Store("")
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(_ context.Context, request *chat.Request) (*chat.Response, error) {
		calls.Add(1)
		encoded, err := json.Marshal(request.Messages)
		if err != nil {
			t.Error(err)
		}
		input.Store(string(encoded))
		return completedTextResponse("observed source inspected"), nil
	})}, chatclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.chatResolver = testChatResolver(client)
	host, api := buildPluginProtocolRuntime(t, cfg)
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.stores.Idempotency, IdempotencyNamespace: cfg.stores.IdempotencyNamespace.String()})
	if err != nil {
		t.Fatal(err)
	}
	r := &poMCPRuntime{t: t, host: host, api: api, endpoint: endpoint}
	t.Cleanup(r.close)
	handler, err := flamehttp.NewRPCHandler(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	opening := make(chan protocol.StartRunResponse, 1)
	var withheld atomic.Bool
	mux := http.NewServeMux()
	for _, route := range flamehttp.Contract().Endpoints {
		if route.Kind != flamehttp.EndpointKindRPC {
			continue
		}
		mux.Handle(route.Method+" "+route.Path, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Error(err)
				return
			}
			request.Body = io.NopCloser(bytes.NewReader(body))
			var envelope struct {
				Method string `json:"method"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				t.Error(err)
				return
			}
			if envelope.Method == "runs.getAcceptance" && withheld.Load() {
				http.Error(w, "fixture: acceptance observation withheld", http.StatusServiceUnavailable)
				return
			}
			if envelope.Method == "runs.start" && withheld.CompareAndSwap(false, true) {
				handler.ServeHTTP(lostOpeningWriter{ResponseWriter: w, opening: opening}, request)
				return
			}
			handler.ServeHTTP(w, request)
		}))
	}
	if err := host.application.callbacks.PublishRPC(mux); err != nil {
		t.Fatal(err)
	}
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "filewatch"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := extension.ParseCapability([]byte(testsupport.FilewatchCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, value any, key string) string {
		t.Helper()
		encoded, err := jsonvalue.Encode(value)
		if err != nil {
			t.Fatal(err)
		}
		result := r.must(delivery.PluginsCall, protocol.CallPluginRequest{InstallationID: installed.ID, Digest: installed.Selected.Digest, Contribution: "filewatch-operations", Contract: contracts.Identity().String(), Method: method, Input: encoded.String(), Target: extension.Target{Scope: "installation", Reference: installed.ID}}, key).(*protocol.CallPluginResponse)
		if result.Failure != nil {
			t.Fatalf("filewatch operation failed: %+v", result.Failure)
		}
		return result.Output
	}
	wait := func(check func() bool) {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for !check() {
			select {
			case <-deadline:
				t.Fatal("installed filewatch state did not arrive")
			case <-time.After(25 * time.Millisecond):
			}
		}
	}
	status := func(phase string, revision int64) bool {
		var value testsupport.FilewatchStatusV1Output
		if err := json.Unmarshal([]byte(call(testsupport.FilewatchStatusV1Method, struct{}{}, "")), &value); err != nil {
			t.Fatal(err)
		}
		if value.Phase == "failed" {
			t.Fatalf("published source failed: %+v", value)
		}
		return value.Phase == phase && value.Revision == revision
	}
	events := func() testsupport.FilewatchEventsV1Output {
		var value testsupport.FilewatchEventsV1Output
		if err := json.Unmarshal([]byte(call(testsupport.FilewatchEventsV1Method, struct{}{}, "")), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	capture := func(id string) testsupport.FilewatchEventV1Output {
		var value testsupport.FilewatchEventV1Output
		if err := json.Unmarshal([]byte(call(testsupport.FilewatchEventV1Method, testsupport.FilewatchEventV1Input{Id: id}, "")), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	workspace := r.must(delivery.WorkspacesResolve, protocol.ResolveWorkspaceRequest{Ref: &protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}}, "").(*protocol.WorkspaceInfo)
	path := filepath.Join(workspace.Ref.Path, "source.txt")
	if err := os.WriteFile(path, []byte("initial baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	draft := testsupport.FilewatchEventV1Output{}.Capture.Trigger
	draft.Title, draft.Workspace, draft.Paths = "Original file trigger", workspace.Ref.Path, []string{"source.txt"}
	draft.Instructions, draft.Provider, draft.Model = "Inspect the original observed source.", cfg.provider, cfg.model
	configured := call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureV1Input{ExpectedRevision: 1, Trigger: &draft}, "configure-source")
	wait(func() bool { return status("watching", 2) })
	if err := os.WriteFile(path, []byte("external source edit"), 0600); err != nil {
		t.Fatal(err)
	}
	var accepted protocol.StartRunResponse
	select {
	case accepted = <-opening:
	case <-time.After(10 * time.Second):
		t.Fatal("external file edit did not reach canonical Run admission")
	}
	original := testsupport.FilewatchEventV1Output{}.Capture
	wait(func() bool {
		values := events()
		if len(values) != 1 {
			return false
		}
		if values[0].Accepted {
			t.Fatal("lost opening invented private acceptance")
		}
		original = capture(values[0].Id).Capture
		return true
	})
	if original.RunId != accepted.RunID || original.Invocation.Origin != string(extension.Automatic) || original.Invocation.Target.Reference != installed.ID || original.Kind != "changed" {
		t.Fatalf("capture differs from canonical opening: %+v %+v", original, accepted)
	}
	call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureV1Input{ExpectedRevision: 2, Trigger: nil}, "disable-source")
	wait(func() bool { return status("disabled", 3) })
	wait(func() bool {
		run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: accepted.RunID}, "").(*protocol.RunRef)
		if run.SessionID != original.SessionId {
			t.Fatal("canonical Run belongs to another captured Session")
		}
		return run.Outcome != nil && run.Outcome.Type == protocol.OutcomeCompleted
	})
	if calls.Load() != 1 || !strings.Contains(input.Load().(string), draft.Instructions) || !strings.Contains(input.Load().(string), "source.txt") {
		t.Fatal("core execution did not consume the original captured input")
	}
	r.close()
	r = poMCPOpen(t, cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	wait(func() bool { values := events(); return len(values) == 1 && values[0].Accepted })
	if values := events(); !reflect.DeepEqual(capture(values[0].Id).Capture, original) || calls.Load() != 1 {
		t.Fatalf("recovery changed intent or reexecuted the accepted Run: %+v calls=%d", values, calls.Load())
	}
	if replay := call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureV1Input{ExpectedRevision: 1, Trigger: &draft}, "configure-source"); replay != configured {
		t.Fatal("configure replay replaced the disabled rule")
	}
	wait(func() bool { return status("disabled", 3) })
	var disabled testsupport.FilewatchGetV1Output
	if err := json.Unmarshal([]byte(call(testsupport.FilewatchGetV1Method, struct{}{}, "")), &disabled); err != nil || disabled.Revision != 3 || disabled.Trigger != nil {
		t.Fatalf("disabled rule changed after receipt replay: %+v %v", disabled, err)
	}
	draft.Instructions = "Inspect the second observed source."
	call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureV1Input{ExpectedRevision: 3, Trigger: &draft}, "rearm-source")
	wait(func() bool { return status("watching", 4) })
	if err := os.WriteFile(path, []byte("second external source edit"), 0600); err != nil {
		t.Fatal(err)
	}
	wait(func() bool {
		values := events()
		return len(values) == 2 && values[0].Accepted && values[0].RunId != original.RunId
	})
	wait(func() bool { return calls.Load() == 2 })
}
