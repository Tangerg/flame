package executionctx

import (
	"context"
	"errors"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
)

type modelCallPublisher interface {
	Start(context.Context, string, flamemodel.Selection) (func(context.Context, error) error, error)
}

type modelCallScope struct {
	publisher     modelCallPublisher
	endpointOwner string
}

func WithModelCalls(ctx context.Context, publisher modelCallPublisher, ownerCallID string) context.Context {
	scope := currentScope(ctx)
	scope.model = modelCallScope{publisher: publisher, endpointOwner: ownerCallID}
	return context.WithValue(ctx, scopeKey{}, scope)
}

func EndpointOwner(ctx context.Context) string {
	return currentScope(ctx).model.endpointOwner
}

func StartIndependentModelCall(ctx context.Context, callID string, selection flamemodel.Selection) (context.Context, func(context.Context, error) error, error) {
	scope := currentScope(ctx)
	if scope.model.publisher == nil {
		return ctx, func(context.Context, error) error { return nil }, nil
	}
	if callID == scope.model.endpointOwner {
		return nil, nil, errors.New("model call: independent invocation cannot claim its executing owner identity")
	}
	finish, err := scope.model.publisher.Start(ctx, callID, selection)
	if err != nil {
		return nil, nil, err
	}
	scope.model.endpointOwner = callID
	return context.WithValue(ctx, scopeKey{}, scope), finish, nil
}
