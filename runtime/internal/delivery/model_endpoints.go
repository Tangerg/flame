package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type endpointObservationUseCases interface {
	List(context.Context, string, string, pagination.RequestedLimit) (pagination.Page[modelref.EndpointObservation], error)
}

func (s *Handler) ListModelEndpoints(ctx context.Context, in protocol.ListModelEndpointsRequest) (*protocol.Page[protocol.ModelEndpointInvocation], error) {
	if s.modelEndpoints == nil {
		return nil, protocol.NewFailure(protocol.ErrCapabilityNotNeg, "model endpoint observations are unavailable")
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
		value, err := presentModelEndpoint(row)
		if err != nil {
			return nil, err
		}
		values[index] = value
	}
	return protocol.NewPageWithCursor(values, page.NextCursor), nil
}

func presentModelEndpoint(observation modelref.EndpointObservation) (protocol.ModelEndpointInvocation, error) {
	if err := observation.Validate(); err != nil {
		return protocol.ModelEndpointInvocation{}, err
	}
	row := observation.Snapshot()
	if err := row.RequestJSON.Validate(); err != nil {
		return protocol.ModelEndpointInvocation{}, err
	}
	value := protocol.ModelEndpointInvocation{CostUSD: row.CostUSD, ID: row.Invocation.ID, Kind: protocol.ModelEndpointKind(row.Invocation.Kind), Caller: row.Invocation.Caller, Provider: row.Invocation.Provider, Model: row.Invocation.Model, SessionID: row.SessionID, RunID: row.RunID, SegmentID: row.SegmentID, Request: row.RequestJSON.String(), Failure: row.Failure, State: protocol.ModelInvocationState(row.State), StartedAt: row.Invocation.StartedAt, SettledAt: row.FinishedAt}
	if observation.HasResponse() {
		if err := row.ResponseJSON.Validate(); err != nil {
			return protocol.ModelEndpointInvocation{}, err
		}
		value.Response = row.ResponseJSON.String()
	}
	return value, nil
}
