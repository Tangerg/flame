package delivery

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
)

const SessionsTrajectory Name = "sessions.trajectory"

func registerSessionTrajectory(registry *Registry) {
	registry.query(MethodMeta{
		Name: SessionsTrajectory, GoName: "ListSessionTrajectory",
		Materializes: []Name{RunsList, ItemsList, modelInvocationsList, modelInvocationsEndpoints},
		Errors:       []string{protocol.ErrSessionNotFound.Error()},
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "includeDescendants", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureChildRuns},
		}},
	}, func(service interface {
		ListSessionTrajectory(context.Context, protocol.ListSessionTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error)
	}, ctx context.Context, request protocol.ListSessionTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error) {
		return service.ListSessionTrajectory(ctx, request)
	})
}

func (s *Handler) ListSessionTrajectory(ctx context.Context, in protocol.ListSessionTrajectoryRequest) (*protocol.Page[protocol.TrajectoryEntry], error) {
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
	switch source := row.Source.(type) {
	case sessions.TrajectoryRun:
		entry.Type, entry.Run = protocol.TrajectoryEntryRun, new(presentRun(source.Run))
	case sessions.TrajectoryModel:
		model, err := presentModelInvocation(source.RunID, source.Invocation)
		if err != nil {
			return protocol.TrajectoryEntry{}, err
		}
		entry.Type, entry.Model = protocol.TrajectoryEntryModel, &model
	case sessions.TrajectoryEndpoint:
		value, err := presentModelEndpoint(source.Observation)
		if err != nil {
			return protocol.TrajectoryEntry{}, err
		}
		entry.Type, entry.Endpoint = protocol.TrajectoryEntryEndpoint, &value
	case sessions.TrajectoryItem:
		entry.Type, entry.Item = protocol.TrajectoryEntryItem, new(presentItem(source.Item))
	default:
		return protocol.TrajectoryEntry{}, fmt.Errorf("delivery: unknown trajectory source %T", source)
	}
	return entry, nil
}
