package models

import (
	"context"
	"errors"
	"strconv"

	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/samber/lo"
)

type endpointReader interface {
	PageEndpointInvocations(context.Context, string, int64, string, int) ([]modelref.EndpointObservation, error)
}

type EndpointObservations struct{ reader endpointReader }

func NewEndpointObservations(reader endpointReader) (*EndpointObservations, error) {
	if lo.IsNil(reader) {
		return nil, errors.New("models: endpoint observation reader is required")
	}
	return &EndpointObservations{reader: reader}, nil
}

func (r *EndpointObservations) List(ctx context.Context, parent, cursor string, limit pagination.RequestedLimit) (pagination.Page[modelref.EndpointObservation], error) {
	var zero pagination.Page[modelref.EndpointObservation]
	if err := modelref.ValidateInvocationID(parent); err != nil {
		return zero, err
	}
	const namespace = "model-endpoints"
	filters := []string{parent}
	anchor, err := pagination.Decode(cursor, namespace, filters)
	if err != nil {
		return zero, err
	}
	var before int64
	var beforeID string
	if len(anchor) > 0 {
		if len(anchor) != 2 {
			return zero, pagination.ErrInvalidCursor
		}
		before, err = strconv.ParseInt(anchor[0], 10, 64)
		if err != nil || anchor[1] == "" {
			return zero, pagination.ErrInvalidCursor
		}
		beforeID = anchor[1]
	}
	size, err := limit.Resolve(100)
	if err != nil {
		return zero, err
	}
	rows, err := r.reader.PageEndpointInvocations(ctx, parent, before, beforeID, size+1)
	if err != nil {
		return zero, err
	}
	return pagination.PageOf(rows, size, namespace, filters, func(row modelref.EndpointObservation) []string {
		record := row.Snapshot()
		return []string{strconv.FormatInt(record.Invocation.StartedAt.UnixNano(), 10), record.Invocation.ID}
	})
}
