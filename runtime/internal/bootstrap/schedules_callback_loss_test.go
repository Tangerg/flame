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

	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	scheduledomain "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	schedulesqlite "github.com/Tangerg/flame/runtime/internal/plugins/schedules/sqlite"
	"github.com/Tangerg/flame/runtime/protocol"
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

func TestSchedulesReconcilesLostCoreOpeningBeforePrivateAcknowledgement(t *testing.T) {
	cfg := poMCPConfig(t)
	host, api := buildPluginProtocolRuntime(t, cfg)
	endpoint, err := delivery.NewEndpoint(api, delivery.EndpointConfig{Lifetime: t.Context(), IdempotencyStore: cfg.Stores.Idempotency, IdempotencyNamespace: cfg.Stores.IdempotencyNamespace.String()})
	if err != nil {
		_ = host.Close()
		t.Fatal(err)
	}
	r := &poMCPRuntime{t: t, host: host, api: api, endpoint: endpoint}
	t.Cleanup(r.close)
	rpc, err := flamehttp.NewRPCHandler(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	opening := make(chan protocol.StartRunResponse, 1)
	var withheld atomic.Bool
	mux := http.NewServeMux()
	for _, route := range flamehttp.Contract().Endpoints {
		if route.Kind == flamehttp.EndpointKindRPC {
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
				if envelope.Method == "runs.start" && withheld.CompareAndSwap(false, true) {
					rpc.ServeHTTP(lostOpeningWriter{ResponseWriter: w, opening: opening}, request)
					return
				}
				rpc.ServeHTTP(w, request)
			}))
		}
	}
	if err := host.application.callbacks.PublishRPC(mux); err != nil {
		t.Fatal(err)
	}
	installed := poMCPActivate(r, filepath.Join(firstPartyPackages, "schedules"))
	server := &protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: installed.ID}, Name: "schedules"}
	ready := func(tools []protocol.MCPTool) bool { return len(tools) == 3 }
	r.poMCPTools(server, ready)
	saved := readScheduleCreate(r, scheduleCreateRequest(r, scheduledomain.Draft{Enabled: true, Title: "original", Instructions: "captured input", Cron: "0 0 1 1 *", CWD: cfg.DefaultWorkspacePath}), "create")
	request := scheduleRunRequest(r, saved.ID)
	lost := r.endpoint.Invoke(t.Context(), delivery.PluginsCall, request, delivery.Options{IdempotencyKey: "lost-core", RequestMeta: protocol.RequestMeta{ProtocolVersion: protocol.ProtocolVersion}})
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
	db, err := schedulesqlite.Open(t.Context(), filepath.Join(cfg.Stores.DataDirectory, "plugins", "data", installed.ID, "schedules.db"))
	if err != nil {
		t.Fatal(err)
	}
	pending, readErr := schedulesqlite.NewScheduleStore(db).PendingManual(t.Context(), "", 1)
	if err := errors.Join(readErr, db.Close()); err != nil || len(pending) != 1 || pending[0].RunID() != accepted.RunID || pending[0].SessionID() != current.SessionID {
		t.Fatalf("Core acceptance did not leave original private pending work: %+v, %v", pending, err)
	}
	r.close()
	r = poMCPOpen(t, cfg)
	r.must(delivery.SessionsDelete, protocol.DeleteSessionRequest{SessionID: current.SessionID}, "retire-accepted-material")
	r.close()
	r = poMCPOpen(t, cfg)
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
