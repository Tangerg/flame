package sessions

import (
	"context"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
)

type queryModelInvocationReader interface {
	PageModelInvocations(context.Context, identity.RunID, int64, string, int) ([]runs.ModelInvocationCommit, error)
}

func (c *QueryCoordinator) ListModelInvocationPage(ctx context.Context, runID identity.RunID, cursor string, limit pagination.RequestedLimit) (pagination.Page[runs.ModelInvocationCommit], error) {
	const namespace = "model-invocations"
	if err := runID.Validate(); err != nil {
		return pagination.Page[runs.ModelInvocationCommit]{}, err
	}
	filters := []string{runID.String()}
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
	size, err := limit.Resolve(modelInvocationPageLimit)
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
