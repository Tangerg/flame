package runtimeadapter

import (
	"context"
	"fmt"
	"slices"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/adapter/runtimeprofile"
	"github.com/Tangerg/flame/cli/internal/domain/agent"
	cliidentity "github.com/Tangerg/flame/cli/internal/domain/identity"
)

type runCatalogBinding interface {
	GetRun(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error)
	ListRuns(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error)
}

func (r *Connection) GetRun(ctx context.Context, runID string) (agent.Run, error) {
	if err := cliidentity.ValidateRun(runID); err != nil {
		return agent.Run{}, fmt.Errorf("get run: %w", err)
	}
	value, err := r.runCatalog.GetRun(ctx, protocol.GetRunRequest{RunID: runID}, r.callOptions())
	if err != nil {
		return agent.Run{}, classifyError(err)
	}
	if value == nil {
		return agent.Run{}, runtimeContractViolation("get run returned nil")
	}
	projected, err := projectRun(*value)
	if err != nil {
		return agent.Run{}, runtimeContractViolation("get run returned an invalid run: %v", err)
	}
	if projected.ID != runID {
		return agent.Run{}, runtimeContractViolation("get run returned id %q for %q", projected.ID, runID)
	}
	return projected, nil
}

func (r *Connection) ListRuns(ctx context.Context, query agent.RunQuery) (agent.RunPage, error) {
	if err := query.Validate(); err != nil {
		return agent.RunPage{}, err
	}
	if query.IncludeDescendants {
		if err := r.requireFeature(runtimeprofile.FeatureSubagents); err != nil {
			return agent.RunPage{}, err
		}
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return agent.RunPage{}, err
	}
	if err := validateRequestCursor("list runs", query.Cursor); err != nil {
		return agent.RunPage{}, err
	}
	var statuses []protocol.RunStatus
	if len(query.Statuses) != 0 {
		statuses = make([]protocol.RunStatus, len(query.Statuses))
		for index, status := range query.Statuses {
			statuses[index] = protocol.RunStatus(status)
		}
	}
	page, err := r.runCatalog.ListRuns(ctx, protocol.ListRunsRequest{
		SessionID: query.SessionID, Statuses: statuses, IncludeDescendants: query.IncludeDescendants,
		PageQuery: protocol.PageQuery{Cursor: query.Cursor, Limit: protocolPositiveInt(limit)},
	}, r.callOptions())
	if err != nil {
		return agent.RunPage{}, classifyError(err)
	}
	return projectRunPage(page, query, limit)
}

func projectRunPage(page *protocol.Page[protocol.RunRef], query agent.RunQuery, limit int) (agent.RunPage, error) {
	if page == nil {
		return agent.RunPage{}, runtimeContractViolation("list runs returned a nil page")
	}
	if len(page.Data) > limit {
		return agent.RunPage{}, runtimeContractViolation("list runs returned %d rows for limit %d", len(page.Data), limit)
	}
	if err := validateContinuationCursor("list runs", query.Cursor, page.NextCursor); err != nil {
		return agent.RunPage{}, err
	}
	projected := agent.RunPage{Items: make([]agent.Run, 0, len(page.Data)), NextCursor: page.NextCursor}
	for _, value := range page.Data {
		run, err := projectRun(value)
		if err != nil {
			return agent.RunPage{}, runtimeContractViolation("list runs returned an invalid run: %v", err)
		}
		if query.SessionID != "" && run.SessionID != query.SessionID {
			return agent.RunPage{}, runtimeContractViolation("list runs for session %q returned run %q from %q", query.SessionID, run.ID, run.SessionID)
		}
		if len(query.Statuses) != 0 && !slices.Contains(query.Statuses, run.Status) {
			return agent.RunPage{}, runtimeContractViolation("list runs returned run %q with unrequested status %q", run.ID, run.Status)
		}
		if !query.IncludeDescendants && !run.Lineage.IsRoot() {
			return agent.RunPage{}, runtimeContractViolation("list root runs returned child %q", run.ID)
		}
		projected.Items = append(projected.Items, run)
	}
	if err := projected.Validate(); err != nil {
		return agent.RunPage{}, runtimeContractViolation("list runs returned an invalid projection: %v", err)
	}
	return projected, nil
}
