package sessions

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
)

type modelInvocationPages struct {
	calls           int
	beforeCallID    string
	beforeStartedAt int64
	limit           int
}

func (m *modelInvocationPages) PageModelInvocations(_ context.Context, _ string, beforeStartedAt int64, beforeCallID string, limit int) ([]runs.ModelInvocationCommit, error) {
	m.calls++
	m.beforeCallID = beforeCallID
	m.beforeStartedAt = beforeStartedAt
	m.limit = limit
	if beforeCallID != "" {
		return []runs.ModelInvocationCommit{invocation("call_a")}, nil
	}
	return []runs.ModelInvocationCommit{invocation("call_c"), invocation("call_b"), invocation("call_a")}, nil
}

func TestModelInvocationPageKeepsOpaqueCursorBoundToItsRun(t *testing.T) {
	reader := &modelInvocationPages{}
	query := newQueryCoordinator(t, QueryDependencies{ModelInvocations: reader})
	limit, err := pagination.NewLimit(2)
	if err != nil {
		t.Fatal(err)
	}
	first, err := query.ListModelInvocationPage(t.Context(), "run_first", "", limit)
	if err != nil || len(first.Rows) != 2 || first.NextCursor == "" || reader.limit != 3 {
		t.Fatalf("first page = %+v, %v; limit=%d", first, err, reader.limit)
	}
	second, err := query.ListModelInvocationPage(t.Context(), "run_first", first.NextCursor, limit)
	if err != nil || len(second.Rows) != 1 || second.NextCursor != "" || reader.beforeCallID != "call_b" || reader.beforeStartedAt != time.Unix(10, 0).UnixNano() {
		t.Fatalf("second page = %+v, %v; reader=%+v", second, err, reader)
	}
	_, err = query.ListModelInvocationPage(t.Context(), "run_other", first.NextCursor, limit)
	if !errors.Is(err, pagination.ErrInvalidCursor) || reader.calls != 2 {
		t.Fatalf("foreign cursor = %v, reads=%d", err, reader.calls)
	}
}

func invocation(id string) runs.ModelInvocationCommit {
	return runs.ModelInvocationCommit{CallID: id, SegmentID: "seg_attempt", State: runs.ModelInvocationCompleted, StartedAt: time.Unix(10, 0), FinishedAt: time.Unix(11, 0)}
}
