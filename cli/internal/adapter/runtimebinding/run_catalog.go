package runtimebinding

import (
	"context"
	"fmt"
	"slices"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type runCatalogBinding interface {
	GetRun(context.Context, protocol.GetRunRequest, flameruntime.CallOptions) (*protocol.RunRef, error)
	ListRuns(context.Context, protocol.ListRunsRequest, flameruntime.CallOptions) (*protocol.Page[protocol.RunRef], error)
}

func (r *Connection) GetRun(ctx context.Context, runID string) (protocol.RunRef, error) {
	if err := protocol.ValidateRunID(runID); err != nil {
		return protocol.RunRef{}, fmt.Errorf("get run: %w", err)
	}
	value, err := r.runCatalog.GetRun(ctx, protocol.GetRunRequest{RunID: runID}, r.callOptions())
	if err != nil {
		return protocol.RunRef{}, classifyError(err)
	}
	if value == nil {
		return protocol.RunRef{}, runtimeContractViolation("get run returned nil")
	}
	if err := protocol.ValidateWireTree(*value); err != nil {
		return protocol.RunRef{}, runtimeContractViolation("get run returned an invalid run: %v", err)
	}
	if value.ID != runID {
		return protocol.RunRef{}, runtimeContractViolation("get run returned id %q for %q", value.ID, runID)
	}
	return *value, nil
}

func (r *Connection) ListRuns(ctx context.Context, query agent.RunQuery) (protocol.Page[protocol.RunRef], error) {
	if err := query.Validate(); err != nil {
		return protocol.Page[protocol.RunRef]{}, err
	}
	if query.IncludeDescendants {
		if err := r.requireFeature(protocol.FeatureSubagents); err != nil {
			return protocol.Page[protocol.RunRef]{}, err
		}
	}
	limit, err := query.PageSize.Rows()
	if err != nil {
		return protocol.Page[protocol.RunRef]{}, err
	}
	if err := validateRequestCursor("list runs", query.Cursor); err != nil {
		return protocol.Page[protocol.RunRef]{}, err
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
		return protocol.Page[protocol.RunRef]{}, classifyError(err)
	}
	return projectRunPage(page, query, limit)
}

func projectRunPage(page *protocol.Page[protocol.RunRef], query agent.RunQuery, limit int) (protocol.Page[protocol.RunRef], error) {
	if page == nil {
		return protocol.Page[protocol.RunRef]{}, runtimeContractViolation("list runs returned a nil page")
	}
	if len(page.Data) > limit {
		return protocol.Page[protocol.RunRef]{}, runtimeContractViolation("list runs returned %d rows for limit %d", len(page.Data), limit)
	}
	if err := validateContinuationCursor("list runs", query.Cursor, page.NextCursor); err != nil {
		return protocol.Page[protocol.RunRef]{}, err
	}
	seen := make(map[string]struct{}, len(page.Data))
	for index, run := range page.Data {
		if err := protocol.ValidateWireTree(run); err != nil {
			return protocol.Page[protocol.RunRef]{}, runtimeContractViolation("list runs returned an invalid run: %v", err)
		}
		if query.SessionID != "" && run.SessionID != query.SessionID {
			return protocol.Page[protocol.RunRef]{}, runtimeContractViolation("list runs for session %q returned run %q from %q", query.SessionID, run.ID, run.SessionID)
		}
		if len(query.Statuses) != 0 && !slices.Contains(query.Statuses, run.Status) {
			return protocol.Page[protocol.RunRef]{}, runtimeContractViolation("list runs returned run %q with unrequested status %q", run.ID, run.Status)
		}
		if !query.IncludeDescendants && run.ParentRunID != "" {
			return protocol.Page[protocol.RunRef]{}, runtimeContractViolation("list root runs returned child %q", run.ID)
		}
		if index > 0 {
			previous := page.Data[index-1]
			if run.CreatedAt.After(previous.CreatedAt) ||
				(run.CreatedAt.Equal(previous.CreatedAt) && run.ID > previous.ID) {
				return protocol.Page[protocol.RunRef]{}, runtimeContractViolation(
					"list runs returned run %q out of newest-first order after %q", run.ID, previous.ID,
				)
			}
		}
		if _, exists := seen[run.ID]; exists {
			return protocol.Page[protocol.RunRef]{}, runtimeContractViolation("list runs repeats id %q", run.ID)
		}
		seen[run.ID] = struct{}{}
	}
	return *page, nil
}
