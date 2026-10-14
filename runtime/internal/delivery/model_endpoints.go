package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/protocol"
)

type endpointObservationUseCases interface {
	List(context.Context, string, string, pagination.RequestedLimit) (pagination.Page[modelref.EndpointObservation], error)
}

func (s *Handler) ListModelEndpoints(ctx context.Context, in protocol.ListModelEndpointsRequest) (*protocol.Page[protocol.ModelEndpointInvocation], error) {
	if s.modelEndpoints == nil {
		return nil, NewFailure(protocol.ErrCapabilityNotNeg, "model endpoint observations are unavailable")
	}
	limit, err := requestedPageLimit(in.Limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	page, err := s.modelEndpoints.List(ctx, in.ParentCallID, in.Cursor, limit)
	if err != nil {
		return nil, wirePageError(err)
	}
	values := make([]protocol.ModelEndpointInvocation, len(page.Rows))
	for index, row := range page.Rows {
		if err := row.RequestJSON.Validate(); err != nil {
			return nil, err
		}
		value := protocol.ModelEndpointInvocation{ID: row.Invocation.ID, Caller: row.Invocation.Caller, Provider: row.Invocation.Provider, Model: row.Invocation.Model, Request: row.RequestJSON.String(), Failure: row.Failure, State: protocol.ModelInvocationState(row.State), StartedAt: row.Invocation.StartedAt, SettledAt: row.FinishedAt}
		if row.Response != nil {
			if err := row.ResponseJSON.Validate(); err != nil {
				return nil, err
			}
			value.Response = row.ResponseJSON.String()
		}
		values[index] = value
	}
	return protocol.NewPageWithCursor(values, page.NextCursor), nil
}
