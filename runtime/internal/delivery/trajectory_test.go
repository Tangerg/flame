package delivery

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
)

type emptyTrajectoryReader struct{}

func (emptyTrajectoryReader) PageTrajectory(context.Context, string, bool, *sessions.TrajectoryPosition, int) ([]sessions.TrajectoryEntry, error) {
	return nil, nil
}

func TestTrajectoryUnknownInvocationPreservesObservationWithoutTimingEstimates(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	entry, err := presentTrajectoryEntry(sessions.TrajectoryEntry{
		OccurredAt: at,
		Model: &sessions.TrajectoryModelInvocation{RunID: "run_unknown", ModelInvocationCommit: runs.ModelInvocationCommit{
			CallID: "call_unknown", SegmentID: "seg_unknown", State: runs.ModelInvocationUnknown,
			StartedAt: at, FinishedAt: at.Add(24 * time.Hour),
		}},
	})
	if err != nil || entry.Type != protocol.TrajectoryEntryModel || entry.Run != nil || entry.Item != nil ||
		entry.Model.State != protocol.ModelInvocationUnknown || entry.Model.Usage != nil ||
		entry.Model.FirstOutputLatencyMillis != nil || !entry.OccurredAt.Equal(at) {
		t.Fatalf("unknown attempt projection = %+v, %v", entry, err)
	}
}

func TestTrajectoryHandlerUsesAlreadyAdmittedRequest(t *testing.T) {
	handler := &Handler{queries: mustQueryCoordinator(sessions.QueryDependencies{})}
	page, err := handler.ListSessionTrajectory(t.Context(), protocol.ListSessionTrajectoryRequest{SessionID: "ses_1", IncludeDescendants: true})
	if err != nil || page == nil || len(page.Data) != 0 {
		t.Fatalf("admitted descendant read = %+v, %v", page, err)
	}
}

type trajectoryCapabilityTarget struct {
	enabled  bool
	requests []protocol.ListSessionTrajectoryRequest
}

func (s *trajectoryCapabilityTarget) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{
		Features: map[string]protocol.FeatureCapability{
			protocol.FeatureSubagents: {Enabled: s.enabled, ClientOptIn: true},
		},
	}}, nil
}

func (s *trajectoryCapabilityTarget) ListSessionTrajectory(_ context.Context, request protocol.ListSessionTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error) {
	s.requests = append(s.requests, request)
	return protocol.NewPage([]protocol.TrajectoryEntry{}), nil
}

func TestTrajectoryEndpointAdmitsOnlyNegotiatedDescendantRequests(t *testing.T) {
	for _, descendants := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			for _, supported := range []bool{false, true} {
				t.Run(fmt.Sprintf("descendants=%v/runtime=%v/client=%v", descendants, enabled, supported), func(t *testing.T) {
					target := &trajectoryCapabilityTarget{enabled: enabled}
					endpoint := mustNewEndpoint(t, target, EndpointConfig{})
					request := protocol.ListSessionTrajectoryRequest{SessionID: "ses_1", IncludeDescendants: descendants}
					result := endpoint.Invoke(t.Context(), SessionsTrajectory, request, Options{RequestMeta: protocol.RequestMeta{
						ClientCapabilities: &protocol.ClientCapabilities{Features: map[string]protocol.FeaturePreference{
							protocol.FeatureSubagents: {Enabled: supported},
						}},
					}})
					if descendants && (!enabled || !supported) {
						assertSubagentCapabilityGap(t, "sessions.trajectory", result.Failure)
						if result.Value != nil || len(target.requests) != 0 {
							t.Fatalf("refused read reached handler: %+v, %+v", result, target.requests)
						}
						return
					}
					if result.Failure != nil || result.Value == nil || len(target.requests) != 1 || target.requests[0] != request {
						t.Fatalf("admitted read = %+v, requests = %+v", result, target.requests)
					}
				})
			}
		}
	}
}

func TestTrajectoryEndpointPreservesOriginalJSONAndSessionOnlyIdentity(t *testing.T) {
	request, err := jsonvalue.Parse([]byte(`{"metadata":{"wide":9007199254740993}}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := jsonvalue.Parse([]byte(`{"metadata":{"usage":{"inputTokens":9007199254740993}}}`))
	if err != nil {
		t.Fatal(err)
	}
	observation := modelref.EndpointObservation{Invocation: modelref.EndpointInvocation{ID: "endpoint_original", Provider: "anthropic", Model: "captured-model", StartedAt: time.Unix(100, 0).UTC()}, SessionID: "ses_original", State: "failed", Failure: "outer failure", FinishedAt: time.Unix(101, 0).UTC(), Response: &chat.Response{}, RequestJSON: request, ResponseJSON: response}
	row := sessions.TrajectoryEntry{OccurredAt: observation.Invocation.StartedAt, Endpoint: &observation}
	entry, err := presentTrajectoryEntry(row)
	if err != nil || entry.Type != protocol.TrajectoryEntryEndpoint || entry.Endpoint.RunID != "" || entry.Endpoint.SessionID != "ses_original" || entry.Endpoint.Request != request.String() || entry.Endpoint.Response != response.String() || entry.Endpoint.State != protocol.ModelInvocationFailed {
		t.Fatalf("physical evidence changed: %+v %v", entry, err)
	}
	if position := row.Position(); position.Kind != sessions.TrajectoryEndpoint || position.ID != "endpoint_original" {
		t.Fatalf("physical cursor identity changed: %+v", position)
	}
}
