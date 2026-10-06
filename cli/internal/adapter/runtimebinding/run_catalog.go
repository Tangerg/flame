package runtimebinding

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type runCatalogBinding interface {
	GetRun(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error)
	ListRuns(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error)
}

func (r *Connection) GetRun(ctx context.Context, runID string) (conversation.Run, error) {
	if err := protocol.ValidateRunID(runID); err != nil {
		return conversation.Run{}, fmt.Errorf("get run: %w", err)
	}
	value, err := r.runCatalog.GetRun(ctx, protocol.GetRunRequest{RunID: runID}, r.callOptions())
	if err != nil {
		return conversation.Run{}, classifyError(err)
	}
	if value == nil {
		return conversation.Run{}, runtimeContractViolation("get run returned nil")
	}
	projected, err := projectRun(*value)
	if err != nil {
		return conversation.Run{}, runtimeContractViolation("get run returned an invalid run: %v", err)
	}
	if projected.ID != runID {
		return conversation.Run{}, runtimeContractViolation("get run returned id %q for %q", projected.ID, runID)
	}
	return projected, nil
}

func (r *Connection) ListRuns(ctx context.Context, query conversation.RunQuery) (conversation.RunPage, error) {
	if err := query.Validate(); err != nil {
		return conversation.RunPage{}, err
	}
	if query.IncludeDescendants {
		if err := r.requireFeature(protocol.FeatureSubagents); err != nil {
			return conversation.RunPage{}, err
		}
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return conversation.RunPage{}, err
	}
	if err := validateRequestCursor("list runs", query.Cursor); err != nil {
		return conversation.RunPage{}, err
	}
	statuses := slices.Clone(query.Statuses)
	if len(statuses) == 0 {
		statuses = nil
	}
	page, err := r.runCatalog.ListRuns(ctx, protocol.ListRunsRequest{
		SessionID: query.SessionID, Statuses: statuses, IncludeDescendants: query.IncludeDescendants,
		PageQuery: protocol.PageQuery{Cursor: query.Cursor, Limit: protocolPositiveInt(limit)},
	}, r.callOptions())
	if err != nil {
		return conversation.RunPage{}, classifyError(err)
	}
	return projectRunPage(page, query, limit)
}

func projectRunPage(page *protocol.Page[protocol.RunRef], query conversation.RunQuery, limit int) (conversation.RunPage, error) {
	if page == nil {
		return conversation.RunPage{}, runtimeContractViolation("list runs returned a nil page")
	}
	if len(page.Data) > limit {
		return conversation.RunPage{}, runtimeContractViolation("list runs returned %d rows for limit %d", len(page.Data), limit)
	}
	if err := validateContinuationCursor("list runs", query.Cursor, page.NextCursor); err != nil {
		return conversation.RunPage{}, err
	}
	projected := conversation.RunPage{Items: make([]conversation.Run, 0, len(page.Data)), NextCursor: page.NextCursor}
	for _, value := range page.Data {
		run, err := projectRun(value)
		if err != nil {
			return conversation.RunPage{}, runtimeContractViolation("list runs returned an invalid run: %v", err)
		}
		if query.SessionID != "" && run.SessionID != query.SessionID {
			return conversation.RunPage{}, runtimeContractViolation("list runs for session %q returned run %q from %q", query.SessionID, run.ID, run.SessionID)
		}
		if len(query.Statuses) != 0 && !slices.Contains(query.Statuses, run.Status) {
			return conversation.RunPage{}, runtimeContractViolation("list runs returned run %q with unrequested status %q", run.ID, run.Status)
		}
		if !query.IncludeDescendants && !run.Lineage.IsRoot() {
			return conversation.RunPage{}, runtimeContractViolation("list root runs returned child %q", run.ID)
		}
		projected.Items = append(projected.Items, run)
	}
	if err := projected.Validate(); err != nil {
		return conversation.RunPage{}, runtimeContractViolation("list runs returned an invalid projection: %v", err)
	}
	return projected, nil
}
