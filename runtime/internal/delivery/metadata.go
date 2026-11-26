package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type requestMetaKey struct{}
type afterEventIDKey struct{}

func WithRequestMeta(ctx context.Context, meta protocol.RequestMeta) context.Context {
	return context.WithValue(ctx, requestMetaKey{}, meta.Clone())
}

func RequestMetaFrom(ctx context.Context) (protocol.RequestMeta, bool) {
	meta, ok := ctx.Value(requestMetaKey{}).(protocol.RequestMeta)
	return meta.Clone(), ok
}

func clientCapabilitiesFrom(ctx context.Context) (*protocol.ClientCapabilities, bool) {
	meta, ok := RequestMetaFrom(ctx)
	if !ok || meta.ClientCapabilities == nil {
		return nil, false
	}
	return meta.ClientCapabilities, true
}

func withAfterEventID(ctx context.Context, eventID string) context.Context {
	return context.WithValue(ctx, afterEventIDKey{}, eventID)
}

func afterEventIDFrom(ctx context.Context) string {
	eventID, _ := ctx.Value(afterEventIDKey{}).(string)
	return eventID
}
