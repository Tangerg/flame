package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	netHTTP "net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/approvals"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/application/automation/goals"
	"github.com/Tangerg/flame/runtime/internal/application/automation/schedules"
	"github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/application/workspace/agentmemory"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

const testRuntimeInstanceID = testsupport.RuntimeInstanceID

// fakeRuns supplies deterministic Application results while the real Handler,
// Endpoint, and Router perform admission and protocol translation.
type fakeRuns struct {
	*runs.Coordinator
	canceledRuns   []string
	gotLastEventID string
}

func (*fakeRuns) ReplayRetention() runs.Retention {
	return runs.Retention{MaxEvents: 100, MaxBytes: 1 << 20}
}

func (f *fakeRuns) Cancel(_ context.Context, in runs.CancelCommand) (runs.CancelResult, error) {
	f.canceledRuns = append(f.canceledRuns, in.RunID)
	finishedAt := time.Date(2026, 7, 30, 1, 0, 0, 0, time.UTC)
	value := testsupport.MustRestoreRun(run.Snapshot{
		ID: in.RunID, SessionID: "ses_test", State: run.Canceled,
		CreatedAt: finishedAt.Add(-time.Second), FinishedAt: finishedAt, UpdatedAt: finishedAt,
	})
	return runs.CancelResult{Run: value}, nil
}

type transportMCP struct{ *mcp.Coordinator }

func (*transportMCP) AuthorizationAttemptRetention() time.Duration { return time.Minute }

// Unused use cases retain their concrete method sets; an accidental invocation
// fails immediately instead of silently supplying a successful fake result.
func newTransportHandler(t *testing.T, cfg delivery.HandlerConfig) *delivery.Handler {
	t.Helper()
	cfg.Sessions = &sessions.Coordinator{}
	cfg.MCP = &transportMCP{}
	cfg.Approvals = &approvals.Coordinator{}
	cfg.Models = &models.Coordinator{}
	cfg.Tools = &workspace.DiagnosticTools{}
	cfg.Queries = &sessions.QueryCoordinator{}
	cfg.Usage = &sessions.UsageReporter{}
	cfg.Feedback = &sessions.FeedbackRecorder{}
	cfg.Schedules = &schedules.Coordinator{}
	cfg.ScheduleFiring = &schedules.Firing{}
	cfg.Goals = &goals.Driver{}
	cfg.AgentMemory = &agentmemory.Coordinator{}
	cfg.WorkspaceFiles = &workspace.Files{}
	cfg.WorkspaceVCS = &workspace.VCS{}
	cfg.WorkspaceDiscovery = &workspace.Discovery{}
	cfg.WorkspaceKnowledge = &workspace.Knowledge{}
	cfg.WorkspaceSkills = &workspace.Skills{}
	cfg.WorkspaceHooks = &workspace.Hooks{}
	cfg.WorkspaceWatch = &workspace.GitWatch{}
	cfg.WorkspaceAuthoredWatch = &workspace.AuthoredWatch{}
	cfg.ServerInfo = protocol.ServerInfo{
		Name: "flame-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID,
		DefaultWorkspace: protocol.WorkspaceRef{Path: "/workspace"}, Home: "/home",
	}
	cfg.IdempotencyLimits = protocol.IdempotencyLimits{RetentionSeconds: 60, Namespace: testsupport.IdempotencyNamespace}
	handler, err := delivery.NewHandler(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func newTestServer(t *testing.T) (*httptest.Server, *fakeRuns) {
	t.Helper()
	api := &fakeRuns{}
	return newTestServerFor(t, delivery.HandlerConfig{Runs: api}), api
}

func newTestEndpoint(t *testing.T, handlerConfig delivery.HandlerConfig, config delivery.EndpointConfig) *delivery.Endpoint {
	t.Helper()
	config.Lifetime = t.Context()
	if config.IdempotencyStore == nil {
		config.IdempotencyStore = testsupport.NewIdempotencyStore()
	}
	endpoint, err := delivery.NewEndpoint(newTransportHandler(t, handlerConfig), config)
	if err != nil {
		t.Fatal(err)
	}
	return endpoint
}

// newTestServerFor serves the real delivery pipeline over explicit Application fixtures.
func newTestServerFor(t *testing.T, handlerConfig delivery.HandlerConfig) *httptest.Server {
	t.Helper()
	srv, err := flamehttp.NewServer(flamehttp.Config{
		Endpoint:        newTestEndpoint(t, handlerConfig, delivery.EndpointConfig{IdempotencyNamespace: testsupport.IdempotencyNamespace}),
		Addr:            ":0",
		ServerInfo:      protocol.ServerInfo{Name: "flame-test", Version: "0.0.0", InstanceID: testRuntimeInstanceID},
		ProtocolVersion: testProtocolVersion,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

// decodeErrorCode reads a JSON-RPC error envelope and returns its code.
func decodeErrorCode(t *testing.T, resp *netHTTP.Response) int {
	t.Helper()
	var env struct {
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Error == nil {
		t.Fatalf("expected an error envelope, got none")
	}
	return env.Error.Code
}

// readBody reads the response body into a string for diagnostic t.Fatalf
// messages.
func readBody(r *netHTTP.Response) string {
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	return buf.String()
}
