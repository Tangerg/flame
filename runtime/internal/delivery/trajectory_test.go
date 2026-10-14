package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
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

func TestTrajectoryDescendantsRequireNegotiatedCapability(t *testing.T) {
	handler := &Handler{queries: mustQueryCoordinator(sessions.QueryDependencies{})}
	if _, err := handler.ListSessionTrajectory(t.Context(), protocol.ListSessionTrajectoryRequest{SessionID: "ses_1", IncludeDescendants: true}); !errors.Is(err, protocol.ErrCapabilityNotNeg) {
		t.Fatalf("descendant read without capability = %v", err)
	}
}

func TestTrajectoryEndpointPreservesOriginalJSONAndSessionOnlyIdentity(t *testing.T) {
	request, err := jsoninput.Parse([]byte(`{"metadata":{"wide":9007199254740993}}`))
	if err != nil {
		t.Fatal(err)
	}
	response, err := jsoninput.Parse([]byte(`{"metadata":{"usage":{"inputTokens":9007199254740993}}}`))
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
