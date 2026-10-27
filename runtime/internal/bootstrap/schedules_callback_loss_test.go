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
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
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
	// Loss fixtures must decorate the original RPC before its single publication.
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
	rpc, err := flamehttp.NewRPCHandler(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	rpc = decorate(rpc)
	mux := http.NewServeMux()
	for _, route := range flamehttp.Contract().Endpoints {
		if route.Kind == flamehttp.EndpointKindRPC {
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
	cfg := poMCPConfig(t)
	r, opening := schedulesWithLostOpening(t, cfg)
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := scheduleToolsReady
	r.poMCPTools(server, ready)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, testsupport.SchedulesCreateV1Input{Enabled: true, Title: "original", Instructions: "captured input", Cron: "0 0 1 1 *", CWD: cfg.defaultWorkspacePath}), "create")
	request := scheduleRunRequest(r, saved.ID)
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "lost-core", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.Version}})
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
	// The private capture is covered by Schedules' own lost-acceptance tests.
	// Here, a restart must recover the same public Run receipt after Core retires it.
	r = reopenPluginRuntime(t, r, &cfg)
	r.must(delivery.SessionsDelete, protocol.DeleteSessionRequest{SessionID: current.SessionID}, "retire-accepted-material")
	r = reopenPluginRuntime(t, r, &cfg)
	r.must(delivery.PluginsSetEnablement, protocol.SetPluginEnablementRequest{InstallationID: installed.ID, Enabled: true}, "reenable")
	r.poMCPTools(server, ready)
	replay := readScheduleRun(r, request, "lost-core")
	if replay.SessionID != current.SessionID || replay.RunID != accepted.RunID {
		t.Fatalf("retired Core acceptance opened another Run: %+v", replay)
	}
	page := readSchedulePage(r, protocol.PageQuery{})
	if len(page.Data) != 1 || page.Data[0].Revision != saved.Revision+1 {
		t.Fatalf("private acknowledgement did not advance exactly once: %+v", page.Data)
	}
	sessions := r.must(delivery.SessionsList, protocol.ListSessionsRequest{}, "").(*protocol.Page[protocol.Session])
	if len(sessions.Data) != 0 {
		t.Fatalf("retired material was recreated: %+v", sessions.Data)
	}
}
