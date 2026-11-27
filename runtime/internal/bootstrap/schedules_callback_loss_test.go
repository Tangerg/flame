package bootstrap

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	runtimehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type lostOpeningWriter struct {
	http.ResponseWriter
	opening chan<- protocol.StartRunResponse
}

func (w lostOpeningWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w lostOpeningWriter) Write(body []byte) (int, error) {
	start := bytes.Index(body, []byte("data:"))
	if start < 0 {
		return 0, errors.New("opening acknowledgement has no SSE data")
	}
	var response struct {
		Result protocol.StartRunResponse `json:"result"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(body[start+5:]), &response); err != nil {
		return 0, err
	}
	w.opening <- response.Result
	return 0, io.ErrClosedPipe
}

func poMCPOpenWithCallbackRPC(t *testing.T, cfg assemblyConfig, decorate func(http.Handler) http.Handler) *poMCPRuntime {
	t.Helper()
	host := assembleTestRuntime(t, cfg)
	r := &poMCPRuntime{t: t, host: host}
	t.Cleanup(r.close)
	api, err := pluginProtocolHandler(host, cfg)
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{
		Lifetime: t.Context(), IdempotencyStore: cfg.stores.Idempotency,
		IdempotencyNamespace: cfg.stores.IdempotencyNamespace.String(), ValidateStorage: cfg.stores.ValidateDirectory,
	})
	if err != nil {
		t.Fatal(err)
	}
	r.api, r.endpoint = api, endpoint
	host.lifetime.delivery = endpoint
	rpc, err := runtimehttp.NewRPCHandler(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	rpc = decorate(rpc)
	mux := http.NewServeMux()
	for _, route := range runtimehttp.Contract().Endpoints {
		if route.Kind == runtimehttp.EndpointKindRPC {
			mux.Handle(route.Method+" "+route.Path, rpc)
		}
	}
	if err := host.application.callbacks.PublishRPC(mux, host.application.modelCallbacks...); err != nil {
		t.Fatal(err)
	}
	if err := host.application.recoverUncertainExecutions(t.Context()); err != nil {
		t.Fatal(err)
	}
	return r
}

func schedulesWithLostOpening(t *testing.T, cfg assemblyConfig) (*poMCPRuntime, <-chan protocol.StartRunResponse) {
	t.Helper()
	opening := make(chan protocol.StartRunResponse, 1)
	var withheld atomic.Bool
	r := poMCPOpenWithCallbackRPC(t, cfg, func(rpc http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
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
			if envelope.Method == "runs.start" && withheld.CompareAndSwap(false, true) {
				rpc.ServeHTTP(lostOpeningWriter{ResponseWriter: w, opening: opening}, request)
				return
			}
			rpc.ServeHTTP(w, request)
		})
	})
	return r, opening
}

func TestSchedulesReconcilesLostCoreOpeningBeforePrivateAcknowledgement(t *testing.T) {
	t.Parallel()
	cfg := poMCPConfig(t)
	r, opening := schedulesWithLostOpening(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := scheduleToolsReady
	project := r.must(delivery.Name("projects.create"), protocol.CreateProjectRequest{Name: "Background recovery"}, "create-project").(*protocol.Project)
	activity := func() *protocol.ProjectBackgroundActivity {
		t.Helper()
		return r.must(delivery.ProjectsGetBackground, protocol.GetProjectRequest{ProjectID: project.ID}, "").(*protocol.ProjectBackgroundActivity)
	}
	policy := activity().Policy
	policy.MaxDailyRuns = 1
	policy = *r.must(delivery.ProjectsSetBackground, protocol.SetProjectBackgroundPolicyRequest{ProjectID: project.ID, Policy: policy}, "limit-project").(*protocol.ProjectBackgroundPolicy)
	r.poMCPTools(server, ready)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, testsupport.SchedulesDraft{Enabled: true, ProjectId: &project.ID, Title: "original", Instructions: "captured input", Cron: "0 0 1 1 *", TimeZone: "UTC", Cwd: cfg.defaultWorkspacePath}), "create")
	request := scheduleRunRequest(r, saved.Id)
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, protocol.CallMetadata{IdempotencyKey: "lost-core", RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
	if lost.Failure == nil {
		t.Fatal("lost Core acknowledgement became private success")
	}
	var accepted protocol.StartRunResponse
	select {
	case accepted = <-opening:
	default:
		t.Fatalf("dispatch returned without witnessing a committed Core opening: %v", lost.Failure)
	}
	current := r.must(delivery.RunsGet, protocol.GetRunRequest{RunID: accepted.RunID}, "").(*protocol.RunRef)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: false}, "disable")
	if state := activity(); state.Admitted != 1 || len(state.Recent) != 1 || state.Recent[0].RunID != accepted.RunID || current.SessionID != state.Recent[0].SessionID {
		t.Fatalf("the uncertain response lost the admitted Project event: %+v", state)
	}
	policy.Paused = true
	r.must(delivery.ProjectsSetBackground, protocol.SetProjectBackgroundPolicyRequest{ProjectID: project.ID, Policy: policy}, "pause-project")
	r = reopenPluginRuntime(t, r, &cfg)
	r.must(delivery.SessionsDelete, protocol.DeleteSessionRequest{SessionID: current.SessionID}, "retire-accepted-material")
	r = reopenPluginRuntime(t, r, &cfg)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "reenable")
	r.poMCPTools(server, ready)
	replay := readScheduleRun(r, request, "lost-core")
	if replay.SessionId != current.SessionID || replay.RunId != accepted.RunID {
		t.Fatalf("retired Core acceptance opened another Run: %+v", replay)
	}
	state := activity()
	if !state.Policy.Paused || state.Admitted != 1 || state.Active != 0 || len(state.Recent) != 1 || state.Recent[0].Status != "retired" {
		t.Fatalf("restart lost the original event, usage or released capacity: %+v", state)
	}
	policy = state.Policy
	policy.Paused = false
	r.must(delivery.ProjectsSetBackground, protocol.SetProjectBackgroundPolicyRequest{ProjectID: project.ID, Policy: policy}, "unpause-project")
	refused := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, protocol.CallMetadata{IdempotencyKey: "exceeds-project-budget", RequestMeta: protocol.RequestMeta{ProtocolVersion: new(protocol.Version)}})
	if refused.Failure == nil && refused.Value.(*protocol.CallPluginResponse).Failure == nil {
		t.Fatal("a new event exceeded the Project's daily admission limit")
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || generatedInteger(t, page.Data[0].Revision) != generatedInteger(t, saved.Revision)+1 {
		t.Fatalf("private acknowledgement did not advance exactly once: %+v", page.Data)
	}
	sessions := r.must(delivery.SessionsList, protocol.ListSessionsRequest{}, "").(*protocol.Page[protocol.Session])
	if len(sessions.Data) != 0 {
		t.Fatalf("retired material was recreated: %+v", sessions.Data)
	}
}
