package delivery

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func wirePageError(err error) error {
	if errors.Is(err, pagination.ErrInvalidCursor) ||
		errors.Is(err, pagination.ErrInvalidCursorMaterial) ||
		errors.Is(err, pagination.ErrCursorTooLarge) ||
		errors.Is(err, pagination.ErrInvalidLimit) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	return err
}

func (s *Handler) ListItems(ctx context.Context, in protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
	limit, err := pagination.ParseLimit(in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	scope, err := itemScopeFromWire(in.Scope)
	if err != nil {
		return nil, err
	}
	if runID, scoped := scope.RunID(); scoped {
		run, found, runErr := s.queries.Run(ctx, runID)
		switch {
		case runErr != nil:
			return nil, runErr
		case !found:
			return nil, protocol.ErrRunNotFound
		case run.Lineage().IsChild():
			if requireFeatureErr := s.requireFeature(ctx, protocol.FeatureChildRuns); requireFeatureErr != nil {
				return nil, requireFeatureErr
			}
		}
	}
	order, err := sequenceOrderFromWire(in.Order)
	if err != nil {
		return nil, err
	}
	page, err := s.queries.ListItemPage(ctx, scope, order, in.Cursor, limit)
	if err != nil {
		return nil, wireItemScopeError(wirePageError(err))
	}
	items := make([]protocol.Item, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, presentItem(item))
	}
	runs := make([]protocol.RunSummary, 0, len(page.Runs))
	for _, run := range page.Runs {
		runs = append(runs, presentRunSummary(run))
	}
	return &protocol.ListItemsResponse{
		Page: *protocol.NewPageWithCursor(items, page.NextCursor),
		Runs: runs,
	}, nil
}

func (s *Handler) GetPlan(ctx context.Context, in protocol.GetPlanRequest) (*protocol.Plan, error) {
	sessionID, parseErr := sessionParameter(in.SessionID)
	if parseErr != nil {
		return nil, parseErr
	}
	state, err := s.queries.PlanState(ctx, sessionID)
	if err != nil {
		return nil, wireItemScopeError(err)
	}
	snapshot := presentStoredPlan(sessionID, state)
	return &snapshot, nil
}

func itemScopeFromWire(scope protocol.ItemListScope) (sessions.ItemScope, error) {
	switch scope.Type {
	case protocol.ItemScopeSession:
		sessionID, err := wireIdentity("scope.sessionId", scope.SessionID, identity.ParseSession)
		if err != nil {
			return sessions.ItemScope{}, err
		}
		return sessions.Items(sessionID), nil
	case protocol.ItemScopeRun:
		runID, err := wireIdentity("scope.runId", scope.RunID, identity.ParseRun)
		if err != nil {
			return sessions.ItemScope{}, err
		}
		if scope.IncludeDescendants {
			return sessions.RunTreeItems(runID), nil
		}
		return sessions.RunItems(runID), nil
	default:
		return sessions.ItemScope{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("scope.type must be %q or %q", protocol.ItemScopeSession, protocol.ItemScopeRun))
	}
}

func sequenceOrderFromWire(order protocol.ItemOrder) (transcript.SequenceOrder, error) {
	switch order {
	case "", protocol.ItemOrderAsc:
		return transcript.OldestFirst, nil
	case protocol.ItemOrderDesc:
		return transcript.NewestFirst, nil
	default:
		return "", protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown order %q", order))
	}
}

func wireItemScopeError(err error) error {
	switch {
	case errors.Is(err, session.ErrNotFound):
		return protocol.ErrSessionNotFound
	case errors.Is(err, transcript.ErrRunNotFound):
		return protocol.ErrRunNotFound
	default:
		return err
	}
}
