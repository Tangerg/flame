package sessions

import (
	"context"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

type QueryModelInvocationReader interface {
	PageModelInvocations(context.Context, string, int64, string, int) ([]runs.ModelInvocationCommit, error)
}

func (c *QueryCoordinator) ListModelInvocationPage(ctx context.Context, runID, cursor string, limit pagination.RequestedLimit) (pagination.Page[runs.ModelInvocationCommit], error) {
	const namespace = "model-invocations"
	if err := resourceid.ValidateRun(runID); err != nil {
		return pagination.Page[runs.ModelInvocationCommit]{}, err
	}
	filters := []string{runID}
	anchor, err := pagination.Decode(cursor, namespace, filters)
	if err != nil {
		return pagination.Page[runs.ModelInvocationCommit]{}, err
	}
	var beforeStartedAt int64
	var beforeCallID string
	if len(anchor) != 0 {
		if len(anchor) != 2 {
			return pagination.Page[runs.ModelInvocationCommit]{}, pagination.ErrInvalidCursor
		}
		beforeStartedAt, err = strconv.ParseInt(anchor[0], 10, 64)
		if err != nil || anchor[1] == "" {
			return pagination.Page[runs.ModelInvocationCommit]{}, pagination.ErrInvalidCursor
		}
		beforeCallID = anchor[1]
	}
	size, err := limit.Resolve(100)
	if err != nil {
		return pagination.Page[runs.ModelInvocationCommit]{}, err
	}
	rows, err := c.modelInvocations.PageModelInvocations(ctx, runID, beforeStartedAt, beforeCallID, size+1)
	if err != nil {
		return pagination.Page[runs.ModelInvocationCommit]{}, err
	}
	return pagination.PageOf(rows, size, namespace, filters, func(row runs.ModelInvocationCommit) []string {
		return []string{strconv.FormatInt(row.StartedAt.UnixNano(), 10), row.CallID}
	})
}
