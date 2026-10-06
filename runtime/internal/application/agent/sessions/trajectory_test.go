package sessions

import (
	"cmp"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type emptyTrajectoryReader struct{}

func (emptyTrajectoryReader) PageTrajectory(context.Context, string, bool, *TrajectoryPosition, int) ([]TrajectoryEntry, error) {
	return nil, nil
}

type trajectoryPages struct {
	entries []TrajectoryEntry
	limit   int
	reads   int
}

func (p *trajectoryPages) PageTrajectory(_ context.Context, _ string, _ bool, anchor *TrajectoryPosition, limit int) ([]TrajectoryEntry, error) {
	p.limit, p.reads = limit, p.reads+1
	var result []TrajectoryEntry
	for _, entry := range p.entries {
		if anchor != nil && compareTrajectoryPositions(entry.Position(), *anchor) >= 0 {
			continue
		}
		result = append(result, entry)
		if len(result) == limit {
			break
		}
	}
	return result, nil
}

func compareTrajectoryPositions(left, right TrajectoryPosition) int {
	return cmp.Or(cmp.Compare(left.OccurredAt, right.OccurredAt), cmp.Compare(left.Kind, right.Kind), cmp.Compare(left.ID, right.ID))
}

func TestTrajectoryPageBindsItsCursorToSessionAndDescendants(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	reader := &trajectoryPages{}
	for _, id := range []string{"call_c", "call_b", "call_a"} {
		reader.entries = append(reader.entries, TrajectoryEntry{
			OccurredAt: at,
			Model: &TrajectoryModelInvocation{RunID: "run_1", ModelInvocationCommit: runs.ModelInvocationCommit{
				CallID: id, SegmentID: "seg_1", StartedAt: at, State: runs.ModelInvocationStarted,
			}},
		})
	}
	query := newQueryCoordinator(t, QueryDependencies{Trajectory: reader})
	first, err := query.ListTrajectoryPage(t.Context(), "ses_1", true, "", explicitPageLimit(t, 2))
	if err != nil || len(first.Rows) != 2 || first.NextCursor == "" || reader.limit != 3 {
		t.Fatalf("first page = %+v, %v, read limit %d", first, err, reader.limit)
	}
	second, err := query.ListTrajectoryPage(t.Context(), "ses_1", true, first.NextCursor, explicitPageLimit(t, 2))
	if err != nil || len(second.Rows) != 1 || second.Rows[0].Model.CallID != "call_a" || second.NextCursor != "" {
		t.Fatalf("next page = %+v, %v", second, err)
	}
	for _, filter := range []struct {
		session     string
		descendants bool
	}{{"ses_other", true}, {"ses_1", false}} {
		if _, err := query.ListTrajectoryPage(t.Context(), filter.session, filter.descendants, first.NextCursor, pagination.DefaultLimit()); !errors.Is(err, pagination.ErrInvalidCursor) {
			t.Fatalf("foreign cursor returned %v", err)
		}
	}
	if reader.reads != 2 {
		t.Fatalf("foreign cursor reached persistence: reads=%d", reader.reads)
	}
	if _, err := query.ListItemPage(t.Context(), Items("ses_1"), transcript.OldestFirst, first.NextCursor, pagination.DefaultLimit()); !errors.Is(err, pagination.ErrInvalidCursor) {
		t.Fatalf("trajectory cursor used as Item cursor: %v", err)
	}
}
