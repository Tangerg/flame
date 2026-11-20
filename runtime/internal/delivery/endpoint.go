package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/samber/lo"
)

const maxIdempotencyKeyBytes = 255

// Endpoint executes the one Runtime operation catalog independently of any
// transport envelope.
type Endpoint struct {
	target               any
	idempotency          *operation.Replay
	idempotencyNamespace operation.ReplayNamespace
	invocations          *invocationGroup
	validateStorage      func() error
}

// EndpointConfig supplies the durable operation owner and process lifetime.
type EndpointConfig struct {
	ValidateStorage      func() error
	IdempotencyStore     operation.ReplayStore
	IdempotencyNamespace string
	// Lifetime ends every in-flight operation and stream owned by this Runtime
	// instance. Required: accepted streams may outlive their request context and
	// must still have one process-owned cancellation root.
	Lifetime context.Context
}

// NewEndpoint constructs the binding-neutral Runtime delivery endpoint.
func NewEndpoint(target any, config EndpointConfig) (*Endpoint, error) {
	if lo.IsNil(config.Lifetime) {
		return nil, errors.New("delivery endpoint: lifetime is required")
	}
	replay, err := operation.NewReplay(config.IdempotencyStore)
	if err != nil {
		return nil, fmt.Errorf("delivery endpoint: %w", err)
	}
	var namespace operation.ReplayNamespace
	if config.IdempotencyNamespace != "" {
		namespace, err = operation.ParseReplayNamespace(config.IdempotencyNamespace)
		if err != nil {
			return nil, fmt.Errorf("delivery endpoint: idempotency namespace: %w", err)
		}
	}
	return &Endpoint{
		validateStorage:      config.ValidateStorage,
		target:               target,
		idempotency:          replay,
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
	return e.idempotency.FlushPending(ctx)
}

// Invoke validates and executes the named operation through the catalog's
// capability, idempotency and safe-problem policies.
func (e *Endpoint) Invoke(ctx context.Context, name Name, parameters any, options protocol.CallMetadata) protocol.OperationResult {
	if e.validateStorage != nil {
		if err := e.validateStorage(); err != nil {
			return failed(projectError(err))
		}
	}
	ctx, release, admitted := e.invocations.Attach(ctx)
	if !admitted {
		return failed(projectError(context.Canceled))
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	method, failure := e.admit(ctx, name, parameters, options)
	if failure != nil {
		return failed(failure)
	}
	ctx, options, err := e.bind(ctx, method, parameters, options)
	if err != nil {
		return failed(projectError(err))
	}
	// Capability admission belongs to this request, not to the operation's stored
	// outcome: a replay must not serve a result the caller could not have asked
	// for, and a request refused for a missing declaration must not occupy the
	// key a complete one retries with.
	if failure := e.enforceCapabilities(ctx, method.Meta, parameters); failure != nil {
		return failed(failure)
	}
	result := e.dispatch(ctx, method, parameters, options.IdempotencyKey)
	if result.Events == nil {
		return result
	}
	result.Events = ownStream(ctx, result.Events, release)
	release = nil
	return result
}

// admit resolves the method a request names and refuses it before any handler
// or idempotency record can observe it.
func (e *Endpoint) admit(ctx context.Context, name Name, parameters any, options protocol.CallMetadata) (*Method, *protocol.Failure) {
	method, ok := contract.lookup(name)
	if !ok {
		return nil, protocol.NewFailure(protocol.ErrMethodNotFound, fmt.Sprintf("unknown method %q", name))
	}
	if failure := validateOptions(method.Meta, options); failure != nil {
		return nil, failure
	}
	if options.IdempotencyKey != "" && options.IdempotencyNamespace != "" &&
		options.IdempotencyNamespace != e.idempotencyNamespace.String() {
		return nil, protocol.NewFailure(protocol.ErrIdempotencyStoreMismatch, "idempotency namespace does not identify this Runtime store")
	}
	if reflect.TypeOf(parameters) != method.Meta.Params {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("%s parameters have type %T, want %s", name, parameters, method.Meta.Params))
	}
	if err := protocol.ValidateWireTree(parameters); err != nil {
		return nil, protocol.InvalidParameters(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, projectError(err)
	}
	if method.prepare == nil {
		return method, nil
	}
	prepared, err := method.prepare(e.target, ctx, parameters)
	if err != nil {
		return nil, projectError(err)
	}
	if failure := validateOptions(prepared.Meta, options); failure != nil {
		return nil, failure
	}
	return prepared, nil
}

// bind attaches the request's operation identity to its context. Run commands
// carry caller-owned identity into their acceptance transaction, because a key
// created only in the response cannot survive a lost acknowledgement.
func (e *Endpoint) bind(ctx context.Context, method *Method, parameters any, options protocol.CallMetadata) (context.Context, protocol.CallMetadata, error) {
	options.IdempotencyKey = operation.ScopedKey(ctx, options.IdempotencyKey)
	ctx = operation.Bind(ctx, e.idempotencyNamespace.String(), options.IdempotencyKey)
	ctx = WithRequestMeta(ctx, options.RequestMeta)
	ctx = withAfterEventID(ctx, options.AfterEventID)
	if method.Meta.Idempotency != IdempotencyOwnerRun {
		return ctx, options, nil
	}
	fingerprint, err := runCommandFingerprint(method.Meta.Name, parameters, options.RequestMeta.ClientCapabilities)
	if err != nil {
		return nil, options, fmt.Errorf("idempotency: fingerprint operation: %w", err)
	}
	return withCommandIdentity(ctx, commandIdentity{key: options.IdempotencyKey, fingerprint: fingerprint}), options, nil
}

// dispatch routes a keyed call by its method's retry contract: a Run command
// serializes through its opening acknowledgement, a response-replay method
// answers from its first outcome, and every other call reaches the handler.
func (e *Endpoint) dispatch(ctx context.Context, method *Method, parameters any, key string) protocol.OperationResult {
	execute := func() protocol.OperationResult { return e.execute(ctx, method, parameters) }
	switch {
	case key == "":
		return execute()
	case method.Meta.Idempotency == IdempotencyOwnerRun:
		// The durable Run acceptance remains the only command outcome authority;
		// serialization never spans the stream lifetime.
		releaseKey, err := e.idempotency.Serialize(ctx, key)
		if err != nil {
			return failed(projectError(err))
		}
		defer releaseKey()
		return execute()
	case method.Meta.Idempotency == idempotencyReplayResponse:
		return e.replayOperation(ctx, method, parameters, key, execute)
	default:
		return execute()
	}
}

func (e *Endpoint) execute(ctx context.Context, method *Method, parameters any) protocol.OperationResult {
	raw := method.invoke(e.target, ctx, parameters)
	if raw.err != nil {
		return failed(projectError(raw.err))
	}
	if err := method.Meta.ValidateResult(raw.value); err != nil {
		return failed(runtimeProduced("an invalid response", err))
	}
	return protocol.OperationResult{Value: raw.value, Events: validateEvents(ctx, method.Meta.Event, raw.events)}
}

func validateEvents(ctx context.Context, eventType reflect.Type, events iter.Seq2[any, error]) iter.Seq2[any, error] {
	if events == nil {
		return nil
	}
	return func(yield func(any, error) bool) {
		for event, err := range events {
			if err != nil {
				yield(nil, projectError(err))
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
	capabilities, ok := clientCapabilitiesFrom(ctx)
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

func validateOptions(method MethodMeta, options protocol.CallMetadata) *protocol.Failure {
	if err := protocol.ValidateWireTree(options.RequestMeta); err != nil {
		return protocol.InvalidParameters(err)
	}
	version := options.RequestMeta.ProtocolVersion
	if version != "" && version != protocol.Version {
		return protocol.NewFailure(protocol.ErrInvalidProtocolVersion, fmt.Sprintf("protocolVersion %q is unsupported; expected %q", version, protocol.Version))
	}
	if failure := validateIdempotencyOptions(method, options); failure != nil {
		return failure
	}
	return validateReplayCursor(method, options)
}

func validateIdempotencyOptions(method MethodMeta, options protocol.CallMetadata) *protocol.Failure {
	key := options.IdempotencyKey
	switch {
	case runs.ReservedCommandKey(key):
		return protocol.NewFailure(protocol.ErrInvalidParams, "idempotency key belongs to a Runtime workflow")
	case key != "" && !method.Idempotency.Replays():
		return protocol.NewFailure(protocol.ErrInvalidParams, "this operation does not accept an idempotency key")
	case key == "" && method.Idempotency == IdempotencyOwnerRun:
		return protocol.NewFailure(protocol.ErrInvalidParams, "a Run command requires an idempotency key retained by its caller")
	case !utf8.ValidString(key):
		return protocol.NewFailure(protocol.ErrInvalidParams, "idempotency key must be valid UTF-8")
	case len(key) > maxIdempotencyKeyBytes:
		return protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("idempotency key must not exceed %d bytes", maxIdempotencyKeyBytes))
	case options.IdempotencyNamespace == "":
		return nil
	case key == "":
		return protocol.NewFailure(protocol.ErrInvalidParams, "an idempotency namespace requires an idempotency key")
	}
	if _, err := operation.ParseReplayNamespace(options.IdempotencyNamespace); err != nil {
		return protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("idempotency namespace: %v", err))
	}
	return nil
}

func validateReplayCursor(method MethodMeta, options protocol.CallMetadata) *protocol.Failure {
	switch {
	case options.AfterEventID == "":
		return nil
	case method.ReplayCursor != ReplayCursorRun:
		return protocol.NewFailure(protocol.ErrInvalidParams, "this operation does not accept a run replay cursor")
	case method.Operation == OperationCommand && options.IdempotencyKey == "":
		return protocol.NewFailure(protocol.ErrInvalidParams, "a run command replay cursor requires an idempotency key")
	}
	if err := flameidentity.ValidateEventIdentity(options.AfterEventID); err != nil {
		return protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("run replay cursor: %v", err))
	}
	return nil
}

func failed(failure *protocol.Failure) protocol.OperationResult {
	return protocol.OperationResult{Failure: failure}
}

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
	options protocol.CallMetadata,
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
