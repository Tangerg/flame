package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"

	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

const maxIdempotencyKeyBytes = 255

// Options carries binding-neutral per-call metadata. Bindings translate their
// native representation into this value before invoking the Endpoint.
type Options struct {
	RequestMeta          protocol.RequestMeta
	IdempotencyKey       string
	IdempotencyNamespace string
	AfterEventID         string
}

// Result is the binding-neutral outcome of one operation. Value and Events are
// type-erased only inside Runtime; typed bindings restore the catalog's declared
// types before returning to their caller.
type Result struct {
	Value   any
	Events  iter.Seq2[any, error]
	Failure *Failure
}

// Endpoint executes the one Runtime operation catalog independently of any
// transport envelope.
type Endpoint struct {
	target               any
	idempotency          *replayStore
	idempotencyNamespace runtimeidentity.IdempotencyNamespace
	invocations          *invocationGroup
}

// EndpointConfig supplies durable operation mechanisms. A nil IdempotencyStore
// selects a Runtime-instance-local store, useful for tests and non-durable hosts.
type EndpointConfig struct {
	IdempotencyStore     idempotency.Store
	IdempotencyNamespace string
	// Lifetime ends every in-flight operation and stream owned by this Runtime
	// instance. Required: accepted streams may outlive their request context and
	// must still have one process-owned cancellation root.
	Lifetime context.Context
}

// NewEndpoint constructs the binding-neutral Runtime delivery endpoint.
func NewEndpoint(target any, config EndpointConfig) (*Endpoint, error) {
	if dependency.Missing(config.Lifetime) {
		return nil, errors.New("delivery endpoint: lifetime is required")
	}
	if config.IdempotencyStore != nil && dependency.Missing(config.IdempotencyStore) {
		return nil, errors.New("delivery endpoint: supplied idempotency store is nil")
	}
	namespace, _, err := runtimeidentity.ParseOptionalIdempotencyNamespace(config.IdempotencyNamespace)
	if err != nil {
		return nil, fmt.Errorf("delivery endpoint: idempotency namespace: %w", err)
	}
	store := config.IdempotencyStore
	if store == nil {
		store = newMemoryIdempotencyStore()
	}
	return &Endpoint{
		target:               target,
		idempotency:          newReplayStore(store),
		idempotencyNamespace: namespace,
		invocations:          newInvocationGroup(config.Lifetime),
	}, nil
}

// BeginShutdown rejects new calls and cancels every accepted call or stream.
// The Runtime owner follows it with AwaitShutdown before closing dependencies.
func (e *Endpoint) BeginShutdown() {
	if target, ok := e.target.(interface{ beginShutdown() }); ok {
		target.beginShutdown()
	}
	e.invocations.BeginShutdown()
}

// AwaitShutdown waits until every accepted call or stream source has returned,
// then persists every known idempotency outcome before its Store can close. It
// does not begin shutdown so process owners can broadcast all cancellation
// signals before joining any one component.
func (e *Endpoint) AwaitShutdown(ctx context.Context) error {
	if err := e.invocations.AwaitShutdown(ctx); err != nil {
		return err
	}
	return e.idempotency.flushPending(ctx)
}

// Invoke validates and executes the named operation through the catalog's
// capability, idempotency and safe-problem policies.
func (e *Endpoint) Invoke(ctx context.Context, name Name, parameters any, options Options) Result {
	ctx, release, admitted := e.invocations.Attach(ctx)
	if !admitted {
		return failed(ProjectError(context.Canceled))
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	method, ok := contract.lookup(name)
	if !ok {
		return failed(NewFailure(protocol.ErrMethodNotFound, fmt.Sprintf("unknown method %q", name)))
	}
	if err := validateOptions(method.Meta, options); err != nil {
		return failed(err)
	}
	if options.IdempotencyKey != "" && options.IdempotencyNamespace != "" &&
		options.IdempotencyNamespace != e.idempotencyNamespace.String() {
		return failed(NewFailure(
			protocol.ErrIdempotencyStoreMismatch,
			"idempotency namespace does not identify this Runtime store",
		))
	}
	if reflect.TypeOf(parameters) != method.Meta.Params {
		return failed(NewFailure(
			protocol.ErrInvalidParams,
			fmt.Sprintf("%s parameters have type %T, want %s", name, parameters, method.Meta.Params),
		))
	}
	if err := protocol.ValidateWireTree(parameters); err != nil {
		return failed(InvalidParameters(err))
	}
	if err := ctx.Err(); err != nil {
		return failed(ProjectError(err))
	}

	options.IdempotencyKey = operation.ScopedKey(ctx, options.IdempotencyKey)
	ctx = operation.Bind(ctx, e.idempotencyNamespace.String(), options.IdempotencyKey)
	ctx = WithRequestMeta(ctx, options.RequestMeta)
	ctx = withAfterEventID(ctx, options.AfterEventID)
	// A keyed owner-run-stream command carries its acceptance into the opening
	// transaction, so the handler needs the request's command identity. The key is
	// optional: an unkeyed start opens a run with no durable acceptance, exactly as
	// before. Binding only the keyed case keeps that one path un-ambient.
	if method.Meta.Idempotency == IdempotencyOwnerRunStream && options.IdempotencyKey != "" {
		fingerprint, err := operationFingerprint(method.Meta.Name, parameters)
		if err != nil {
			return failed(ProjectError(fmt.Errorf("idempotency: fingerprint operation: %w", err)))
		}
		ctx = withCommandIdentity(ctx, commandIdentity{key: options.IdempotencyKey, fingerprint: fingerprint})
	}
	// Capability admission belongs to this request, not to the operation's stored
	// outcome: a replay must not serve a result the caller could not have asked
	// for, and a request refused for a missing declaration must not occupy the
	// key a complete one retries with.
	if failure := e.enforceCapabilities(ctx, method.Meta, parameters); failure != nil {
		return failed(failure)
	}
	execute := func() Result { return e.execute(ctx, method, parameters) }
	var result Result
	if options.IdempotencyKey == "" || !method.Meta.Idempotency.Replays() || method.Meta.Idempotency.OwnsOutcome() {
		result = execute()
	} else {
		result = e.idempotency.invoke(ctx, method, parameters, options.IdempotencyKey, execute, e.target)
	}
	if result.Events == nil {
		return result
	}
	result.Events = ownStream(ctx, result.Events, release)
	release = nil
	return result
}

func (e *Endpoint) execute(ctx context.Context, method *Method, parameters any) Result {
	raw := method.invoke(e.target, ctx, parameters)
	if raw.err != nil {
		return failed(ProjectError(raw.err))
	}
	if err := method.Meta.ValidateResult(raw.value); err != nil {
		return failed(runtimeProduced("an invalid response", err))
	}
	return Result{Value: raw.value, Events: validateEvents(ctx, method.Meta.Event, raw.events)}
}

func validateEvents(ctx context.Context, eventType reflect.Type, events iter.Seq2[any, error]) iter.Seq2[any, error] {
	if events == nil {
		return nil
	}
	return func(yield func(any, error) bool) {
		for event, err := range events {
			if err != nil {
				yield(nil, ProjectError(err))
				return
			}
			if actual := reflect.TypeOf(event); actual != eventType {
				yield(nil, runtimeProduced(
					"an event with an invalid type",
					fmt.Errorf("event has type %s, want %s", actual, eventType),
				))
				return
			}
			if err := protocol.ValidateWireTree(event); err != nil {
				yield(nil, runtimeProduced("an invalid event", err))
				return
			}
			if !allowsEvent(ctx, event) {
				continue
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}

func allowsEvent(ctx context.Context, event any) bool {
	runEvent, ok := event.(protocol.RunEvent)
	if !ok {
		return true
	}
	capabilities, ok := ClientCapabilitiesFrom(ctx)
	if !ok {
		return true
	}
	for _, excluded := range capabilities.ExcludedEphemeralEvents {
		if excluded == protocol.SuppressibleRunEventType(runEvent.Event.Type) {
			return false
		}
	}
	return true
}

func validateOptions(method MethodMeta, options Options) *Failure {
	if err := protocol.ValidateWireTree(options.RequestMeta); err != nil {
		return InvalidParameters(err)
	}
	version := options.RequestMeta.ProtocolVersion
	if version != "" && version != protocol.ProtocolVersion {
		return NewFailure(
			protocol.ErrInvalidProtocolVersion,
			fmt.Sprintf("protocolVersion %q is unsupported; expected %q", version, protocol.ProtocolVersion),
		)
	}
	if options.IdempotencyKey != "" && !method.Idempotency.Replays() {
		return NewFailure(protocol.ErrInvalidParams, "this operation does not accept an idempotency key")
	}
	if len(options.IdempotencyKey) > maxIdempotencyKeyBytes {
		return NewFailure(protocol.ErrInvalidParams, "idempotency key must not exceed 255 bytes")
	}
	if options.IdempotencyNamespace != "" && options.IdempotencyKey == "" {
		return NewFailure(protocol.ErrInvalidParams, "an idempotency namespace requires an idempotency key")
	}
	if options.IdempotencyNamespace != "" {
		if _, err := runtimeidentity.ParseIdempotencyNamespace(options.IdempotencyNamespace); err != nil {
			return NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("idempotency namespace: %v", err))
		}
	}
	if options.AfterEventID != "" {
		if method.ReplayCursor != ReplayCursorRun {
			return NewFailure(protocol.ErrInvalidParams, "this operation does not accept a run replay cursor")
		}
		if method.Operation == OperationCommand && options.IdempotencyKey == "" {
			return NewFailure(protocol.ErrInvalidParams, "a run command replay cursor requires an idempotency key")
		}
		if err := runtimeidentity.ValidateEventIdentity(options.AfterEventID); err != nil {
			return NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("run replay cursor: %v", err))
		}
	}
	return nil
}

func failed(failure *Failure) Result { return Result{Failure: failure} }

// commandIdentity is the request's idempotency key and fingerprint. It reaches an
// owner-run-stream handler through the context because the handler owns the
// durable acceptance, while the key and fingerprint are the delivery boundary's to
// state. The fingerprint is the one authoritative request fingerprint shared with
// the replay store, so acceptance and replay cannot disagree on request identity.
type commandIdentity struct {
	key         string
	fingerprint string
}

type commandIdentityKey struct{}

func withCommandIdentity(ctx context.Context, identity commandIdentity) context.Context {
	return context.WithValue(ctx, commandIdentityKey{}, identity)
}

func commandIdentityFrom(ctx context.Context) (commandIdentity, bool) {
	identity, ok := ctx.Value(commandIdentityKey{}).(commandIdentity)
	return identity, ok
}

// Call restores the typed result declared by a unary catalog entry.
func (e *Endpoint) Call[Params, Response any](
	ctx context.Context,
	name Name,
	parameters Params,
	options Options,
) (Response, error) {
	var zero Response
	result := e.Invoke(ctx, name, parameters, options)
	if result.Failure != nil {
		return zero, result.Failure
	}
	value, ok := result.ValueAs[Response]()
	if !ok {
		return zero, runtimeProduced(
			"a response with an invalid type",
			fmt.Errorf("response has type %T, want %T", result.Value, zero),
		)
	}
	return value, nil
}
