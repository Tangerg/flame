package delivery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/protocol"
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
