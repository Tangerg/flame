package runtimefixture

import (
	"context"
	"fmt"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

func (r *Runtime) GetRun(ctx context.Context, runID string) (protocol.RunRef, error) {
	if err := context.Cause(ctx); err != nil {
		return protocol.RunRef{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	run := r.runs[runID]
	if run == nil {
		return protocol.RunRef{}, fmt.Errorf("%w: %s", protocol.ErrRunNotFound, runID)
	}
	return projectRun(run), nil
}

func (r *Runtime) ListRuns(ctx context.Context, query agent.RunQuery) (protocol.Page[protocol.RunRef], error) {
	if err := query.Validate(); err != nil {
		return protocol.Page[protocol.RunRef]{}, fmt.Errorf("mock: %w", err)
	}
	if err := context.Cause(ctx); err != nil {
		return protocol.Page[protocol.RunRef]{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]protocol.RunRef, 0, len(r.runOrder))
	for _, runID := range slices.Backward(r.runOrder) {
		run := r.runs[runID]
		if run == nil || (query.SessionID != "" && run.sessionID != query.SessionID) {
			continue
		}
		if len(query.Statuses) != 0 && !slices.Contains(query.Statuses, run.status) {
			continue
		}
		items = append(items, projectRun(run))
	}

	offset, err := pageOffset("run", query.Cursor, len(items))
	if err != nil {
		return protocol.Page[protocol.RunRef]{}, err
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return protocol.Page[protocol.RunRef]{}, fmt.Errorf("mock: %w", err)
	}
	end := min(offset+limit, len(items))
	page := protocol.Page[protocol.RunRef]{Data: slices.Clone(items[offset:end])}
	if end < len(items) {
		page.NextCursor = strconv.Itoa(end)
	}
	return page, nil
}
