package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/protocol"
)

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
	if config.Lifetime == nil {
		return nil, errors.New("delivery endpoint: lifetime is required")
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
	method, ok := contract.lookup(name)
	if !ok {
		release()
		return failed(NewFailure(protocol.ErrMethodNotFound, fmt.Sprintf("unknown method %q", name)))
	}
	if err := validateOptions(method.Meta, options); err != nil {
		release()
		return failed(err)
	}
	if options.IdempotencyKey != "" && options.IdempotencyNamespace != "" &&
		options.IdempotencyNamespace != e.idempotencyNamespace.String() {
		release()
		return failed(NewFailure(
			protocol.ErrIdempotencyStoreMismatch,
			"idempotency namespace does not identify this Runtime store",
		))
	}
	if reflect.TypeOf(parameters) != method.Meta.Params {
		release()
		return failed(NewFailure(
			protocol.ErrInvalidParams,
			fmt.Sprintf("%s parameters have type %T, want %s", name, parameters, method.Meta.Params),
		))
	}
	if err := protocol.ValidateWireTree(parameters); err != nil {
		release()
		return failed(InvalidParameters(err))
	}
	if err := ctx.Err(); err != nil {
		release()
		return failed(ProjectError(err))
	}

	ctx = WithRequestMeta(ctx, options.RequestMeta)
	ctx = withAfterEventID(ctx, options.AfterEventID)
	execute := func() Result { return e.execute(ctx, method, parameters) }
	var result Result
	if options.IdempotencyKey == "" || !method.Meta.Idempotency.Replays() {
		result = execute()
	} else {
		result = e.idempotency.invoke(ctx, method, parameters, options.IdempotencyKey, execute, e.target)
	}
	if result.Events == nil {
		release()
		return result
	}
	result.Events = ownStream(ctx, result.Events, release)
	return result
}

func (e *Endpoint) execute(ctx context.Context, method *Method, parameters any) Result {
	if err := e.enforceCapabilities(ctx, method.Meta, parameters); err != nil {
		return failed(err)
	}
	raw := method.invoke(e.target, ctx, parameters)
	if raw.err != nil {
		return failed(ProjectError(raw.err))
	}
	if err := protocol.ValidateWireTree(raw.value); err != nil {
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
	value, ok := result.Value.(Response)
	if !ok {
		return zero, runtimeProduced(
			"a response with an invalid type",
			fmt.Errorf("response has type %T, want %T", result.Value, zero),
		)
	}
	return value, nil
}

// CallStream restores the typed acknowledgement and event sequence declared by
// a streaming catalog entry.
func (e *Endpoint) CallStream[Params, Ack, Event any](
	ctx context.Context,
	name Name,
	parameters Params,
	options Options,
) (Ack, iter.Seq2[Event, error], error) {
	var zero Ack
	result := e.Invoke(ctx, name, parameters, options)
	if result.Failure != nil {
		return zero, nil, result.Failure
	}
	ack, ok := result.Value.(Ack)
	if !ok {
		return zero, nil, runtimeProduced(
			"an acknowledgement with an invalid type",
			fmt.Errorf("acknowledgement has type %T, want %T", result.Value, zero),
		)
	}
	return ack, restoreEventType[Event](result.Events), nil
}

func restoreEventType[Event any](events iter.Seq2[any, error]) iter.Seq2[Event, error] {
	if events == nil {
		return nil
	}
	return func(yield func(Event, error) bool) {
		for value, err := range events {
			if err != nil {
				var zero Event
				yield(zero, err)
				return
			}
			event, ok := value.(Event)
			if !ok {
				var zero Event
				yield(zero, runtimeProduced(
					"an event with an invalid type",
					fmt.Errorf("event has type %T, want %T", value, zero),
				))
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}
