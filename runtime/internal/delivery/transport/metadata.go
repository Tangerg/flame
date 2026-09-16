package transport

import "context"

// Last-Event-Id is a streaming reconnect cursor — transport metadata, not
// a business param. The transport reads it off the
// wire (the HTTP Last-Event-Id header) and carries it on the
// context with WithLastEventID; the runtime's SubscribeRun reads it with
// LastEventIDFrom to replay a run's retained, replayable backlog from that point.
// Empty — a transport that does not carry it, or a fresh subscribe — attaches at
// the current head and replays nothing. The run journal owns that rule: a stream
// that also replayed its beginning is one a client folds on top of the
// transcript reads it is already making, which applies those events twice.
type lastEventIDKey struct{}

type idempotencyKey struct{}
type idempotencyNamespace struct{}

// WithLastEventID returns ctx carrying the streaming reconnect cursor.
func WithLastEventID(ctx context.Context, id string) context.Context {
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, lastEventIDKey{}, id)
}

// LastEventIDFrom reads the reconnect cursor, "" when unset (attach at head).
func LastEventIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(lastEventIDKey{}).(string)
	return id
}

// WithIdempotencyKey carries the transport-level Idempotency-Key metadata to
// the router without polluting business request DTOs.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
	if key == "" {
		return ctx
	}
	return context.WithValue(ctx, idempotencyKey{}, key)
}

func IdempotencyKeyFrom(ctx context.Context) string {
	key, _ := ctx.Value(idempotencyKey{}).(string)
	return key
}

// WithIdempotencyNamespace carries the caller's expected durable replay-store
// identity beside its idempotency key.
func WithIdempotencyNamespace(ctx context.Context, namespace string) context.Context {
	if namespace == "" {
		return ctx
	}
	return context.WithValue(ctx, idempotencyNamespace{}, namespace)
}

func IdempotencyNamespaceFrom(ctx context.Context) string {
	namespace, _ := ctx.Value(idempotencyNamespace{}).(string)
	return namespace
}
