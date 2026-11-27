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
	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/chatclient"
)

func TestFilewatchInstalledIngressReconcilesLostOpeningAcrossDisableAndRestart(t *testing.T) {
	t.Parallel()
	t.Run("new-work", func(t *testing.T) { testFilewatchRetry(t, false) })
	t.Run("continue-work", func(t *testing.T) { testFilewatchRetry(t, true) })
}

func testFilewatchRetry(t *testing.T, follow bool) {
	t.Helper()
	cfg := poMCPConfig(t)
	var calls atomic.Int32
	var input atomic.Value
	input.Store("")
	releaseFirst := make(chan struct{})
	client, err := chatclient.New(delegateRestartModel{chat.ModelFunc(func(ctx context.Context, request *chat.Request) (*chat.Response, error) {
		first := calls.Add(1) == 1
		encoded, err := json.Marshal(request.Messages)
		if err != nil {
			t.Error(err)
		}
		input.Store(string(encoded))
		if follow && first {
			select {
			case <-releaseFirst:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return completedTextResponse("observed source inspected"), nil
	})})
	if err != nil {
		t.Fatal(err)
	}
	cfg.modelProviders = testChatResolver(client)
	opening := make(chan protocol.StartRunResponse, 1)
	fault := &lostOpeningFault{t: t, opening: opening}
	r := poMCPOpenWithCallbackRPC(t, cfg, fault.wrap)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "filewatch"))
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	contracts, err := plugincontract.ParseCapability([]byte(testsupport.FilewatchCapabilityJSON))
	if err != nil {
		t.Fatal(err)
	}
	call := func(method string, value any, key string) string {
		t.Helper()
		encoded, err := framevalue.EncodeJSON(value)
		if err != nil {
			t.Fatal(err)
		}
		result := r.must(delivery.PluginsCall, protocol.CallPluginRequest{
			Route: protocol.PluginCallRoute{
				InstallationID: installed.ID,
				Digest:         installed.Selected.Digest,
				Contribution:   "filewatch-operations",
				Contract:       contracts.Identity().String(),
				Method:         method,
				Target:         framecall.Target{Scope: extension.ScopeInstallation, Reference: installed.ID},
			},
			Input: encoded.String(),
		}, key).(*protocol.CallPluginResponse)
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
	status := func(phase testsupport.FilewatchStatusPhase, revision int64) bool {
		var value testsupport.FilewatchStatus
		if err := json.Unmarshal([]byte(call(testsupport.FilewatchStatusV1Method, struct{}{}, "")), &value); err != nil {
			t.Fatal(err)
		}
		if value.Phase == testsupport.FilewatchStatusPhaseFailed {
			t.Fatalf("published source failed: %+v", value)
		}
		return value.Phase == phase && generatedInteger(t, value.Revision) == uint64(revision)
	}
	events := func() testsupport.FilewatchEventsV1Output {
		var value testsupport.FilewatchEventsV1Output
		if err := json.Unmarshal([]byte(call(testsupport.FilewatchEventsV1Method, struct{}{}, "")), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	capture := func(id string) testsupport.FilewatchEvent {
		var value testsupport.FilewatchEvent
		if err := json.Unmarshal([]byte(call(testsupport.FilewatchEventV1Method, testsupport.FilewatchEventInput{Id: id}, "")), &value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	workspace := r.must(delivery.WorkspacesResolve, protocol.ResolveWorkspaceRequest{Ref: &protocol.WorkspaceRef{Path: cfg.defaultWorkspacePath}}, "").(*protocol.WorkspaceInfo)
	path := filepath.Join(workspace.Ref.Path, "source.txt")
	if err := os.WriteFile(path, []byte("initial baseline"), 0600); err != nil {
		t.Fatal(err)
	}
	draft := testsupport.FilewatchConfigureV1InputFilewatchDraft{}
	draft.Title, draft.Workspace, draft.Paths = "Original file trigger", workspace.Ref.Path, []string{"source.txt"}
	draft.Instructions = "Inspect the original observed source."
	var existing *protocol.Session
	if follow {
		existing = r.must(delivery.SessionsCreate, protocol.CreateSessionRequest{Title: "Original analysis", Workspace: &workspace.Ref}, "original-work").(*protocol.Session)
		draft.TargetSessionId = &existing.ID
	} else {
		draft.Provider, draft.Model = new(cfg.defaultSelection.Provider()), new(cfg.defaultSelection.Model())
	}
	configured := call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureInput{ExpectedRevision: generatedNumber(t, "1"), Trigger: &draft}, "configure-source")
	wait(func() bool { return status(testsupport.FilewatchStatusPhaseWatching, 2) })
	changed := managedServiceChange(t, r, protocol.TopicPluginDataChanged)
	if err := os.WriteFile(path, []byte("external source edit"), 0600); err != nil {
		t.Fatal(err)
	}
	var accepted protocol.StartRunResponse
	select {
	case accepted = <-opening:
	case <-time.After(10 * time.Second):
		t.Fatal("external file edit did not reach canonical Run admission")
	}
	requireManagedServiceChange(t, changed)
	var original *testsupport.FilewatchCapture
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
	plannedRun := accepted.RunID
	if follow {
		plannedRun = ""
		if original.SessionId != existing.ID {
			t.Fatal("continued source went to another Session")
		}
	}
	if original.RunId != plannedRun || original.Invocation.Origin != string(extension.Automatic) || original.Invocation.Target.Reference != installed.ID || original.Kind != "changed" {
		t.Fatalf("capture differs from canonical opening: %+v %+v", original, accepted)
	}
	call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureInput{ExpectedRevision: generatedNumber(t, "2"), Trigger: nil}, "disable-source")
	wait(func() bool { return status(testsupport.FilewatchStatusPhaseDisabled, 3) })
	if follow {
		run := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: accepted.RunID}, "").(*protocol.RunRef)
		if run.Outcome != nil || calls.Load() != 1 {
			t.Fatalf("disabling observation ended active work: %+v calls=%d", run.Outcome, calls.Load())
		}
		close(releaseFirst)
	}
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
	r = reopenPluginRuntime(t, r, &cfg)
	waitCallContributorState(t, r, installed.ID, protocol.MCPServerConnected)
	wait(func() bool { values := events(); return len(values) == 1 && values[0].Accepted })
	if values := events(); !reflect.DeepEqual(capture(values[0].Id).Capture, original) ||
		values[0].RunId != accepted.RunID || calls.Load() != 1 {
		t.Fatalf("recovery changed intent or reexecuted the accepted Run: %+v calls=%d", values, calls.Load())
	}
	if follow {
		recovered := r.must(delivery.SessionsGet, protocol.GetSessionRequest{SessionID: existing.ID}, "").(*protocol.Session)
		if recovered.Provider != existing.Provider || recovered.Model != existing.Model ||
			!strings.Contains(input.Load().(string), "methodRevision") {
			t.Fatal("continuation changed the Session default or lost its method identity")
		}
	}
	if replay := call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureInput{ExpectedRevision: generatedNumber(t, "1"), Trigger: &draft}, "configure-source"); replay != configured {
		t.Fatal("configure replay replaced the disabled rule")
	}
	wait(func() bool { return status(testsupport.FilewatchStatusPhaseDisabled, 3) })
	var disabled testsupport.FilewatchGetV1OutputFilewatchSnapshot
	if err := json.Unmarshal([]byte(call(testsupport.FilewatchGetV1Method, struct{}{}, "")), &disabled); err != nil || generatedInteger(t, disabled.Revision) != 3 || disabled.Trigger != nil {
		t.Fatalf("disabled rule changed after receipt replay: %+v %v", disabled, err)
	}
	draft.Instructions = "Inspect the second observed source."
	call(testsupport.FilewatchConfigureV1Method, testsupport.FilewatchConfigureInput{ExpectedRevision: generatedNumber(t, "3"), Trigger: &draft}, "rearm-source")
	wait(func() bool { return status(testsupport.FilewatchStatusPhaseWatching, 4) })
	if err := os.WriteFile(path, []byte("second external source edit"), 0600); err != nil {
		t.Fatal(err)
	}
	wait(func() bool {
		values := events()
		return len(values) == 2 && values[0].Accepted && values[0].RunId != accepted.RunID && (!follow || values[0].SessionId == original.SessionId)
	})
	wait(func() bool { return calls.Load() == 2 })
}

// lostOpeningFault loses the response of the first runs.start that crosses the
// callback RPC, and then withholds acceptance reads while the test needs it to.
type lostOpeningFault struct {
	t        *testing.T
	opening  chan protocol.StartRunResponse
	withheld atomic.Bool
}

func (f *lostOpeningFault) wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			f.t.Error(err)
			return
		}
		request.Body = io.NopCloser(bytes.NewReader(body))
		var envelope struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			f.t.Error(err)
			return
		}
		switch {
		case envelope.Method == "runs.getAcceptance" && f.withheld.Load():
			http.Error(w, "fixture: acceptance observation withheld", http.StatusServiceUnavailable)
		case envelope.Method == "runs.start" && f.withheld.CompareAndSwap(false, true):
			handler.ServeHTTP(lostOpeningWriter{ResponseWriter: w, opening: f.opening}, request)
		default:
			handler.ServeHTTP(w, request)
		}
	})
}
