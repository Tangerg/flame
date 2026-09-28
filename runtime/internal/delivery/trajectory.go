package delivery

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/protocol"
)

const SessionsTrajectory Name = "sessions.trajectory"

func registerSessionTrajectory(registry *Registry) {
	registry.query(MethodMeta{
		Name:         SessionsTrajectory,
		Materializes: []Name{RunsList, ItemsList, ModelInvocationsList},
		Errors:       []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureSubagents},
		}},
	}, func(service interface {
		ListSessionTrajectory(context.Context, protocol.ListSessionTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error)
	}, ctx context.Context, request protocol.ListSessionTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error) {
		return service.ListSessionTrajectory(ctx, request)
	})
}

func (s *Handler) ListSessionTrajectory(ctx context.Context, in protocol.ListSessionTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error) {
	if in.IncludeDescendants {
		if err := s.requireFeature(ctx, protocol.FeatureSubagents); err != nil {
			return nil, err
		}
	}
	limit, err := requestedPageLimit(in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	page, err := s.queries.ListTrajectoryPage(ctx, in.SessionID, in.IncludeDescendants, in.Cursor, limit)
	if err != nil {
		return nil, wireItemScopeError(wirePageError(err))
	}
	entries := make([]protocol.TrajectoryEntry, 0, len(page.Rows))
	for _, row := range page.Rows {
		entry, err := presentTrajectoryEntry(row)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return protocol.NewPageWithCursor(entries, page.NextCursor), nil
}

func presentTrajectoryEntry(row sessions.TrajectoryEntry) (protocol.TrajectoryEntry, error) {
	entry := protocol.TrajectoryEntry{OccurredAt: row.OccurredAt}
	switch {
	case row.Run != nil:
		entry.Type, entry.Run = protocol.TrajectoryEntryRun, new(presentRun(*row.Run))
	case row.Model != nil:
		entry.Type, entry.Model = protocol.TrajectoryEntryModel, new(presentModelInvocation(row.Model.RunID, row.Model.ModelInvocationCommit))
	case row.Item != nil:
		entry.Type, entry.Item = protocol.TrajectoryEntryItem, new(presentItem(*row.Item))
	default:
		return protocol.TrajectoryEntry{}, errors.New("delivery: trajectory entry has no source")
	}
	return entry, nil
}
