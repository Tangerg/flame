package delivery

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"reflect"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/samber/lo"
)

type Registry struct {
	byName map[Name]*Method
	names  []Name
}

type Method struct {
	Meta        MethodMeta
	invoke      func(any, context.Context, any) rawResult
	prepare     func(any, context.Context, any) (*Method, error)
	classify    func(any) (OperationKind, error)
	ownedReplay ownedReplay
}

type rawResult struct {
	value  any
	events iter.Seq2[any, error]
	err    error
}

func newRegistry() *Registry {
	return &Registry{byName: make(map[Name]*Method)}
}

func (r *Registry) add(meta MethodMeta, invoke func(any, context.Context, any) rawResult) *Method {
	if err := meta.Validate(); err != nil {
		panic("operation: invalid method registration: " + err.Error())
	}
	if _, exists := r.byName[meta.Name]; exists {
		panic(fmt.Sprintf("operation: method %q is registered twice", meta.Name))
	}
	method := &Method{Meta: cloneMethodMeta(meta), invoke: invoke}
	r.byName[meta.Name] = method
	r.names = append(r.names, meta.Name)
	return method
}

func (m *Method) withOwnedReplay[Capability, Params any](settle func(Capability, context.Context, Params, keyedCall) protocol.OperationResult) {
	if m.Meta.Params != reflect.TypeFor[Params]() || m.Meta.Idempotency != idempotencyReplayResponse {
		panic(fmt.Sprintf("operation: %s cannot settle its own replay for %s", m.Meta.Name, reflect.TypeFor[Params]()))
	}
	m.ownedReplay = func(target any, ctx context.Context, parameters any, call keyedCall) protocol.OperationResult {
		capability, ok := target.(Capability)
		if !ok {
			return failed(projectError(fmt.Errorf("operation: target cannot settle %s", m.Meta.Name)))
		}
		params, ok := parameters.(Params)
		if !ok {
			return failed(projectError(fmt.Errorf("operation: %s received %T parameters", m.Meta.Name, parameters)))
		}
		return settle(capability, ctx, params, call)
	}
}

func (r *Registry) lookup(name Name) (*Method, bool) {
	method, ok := r.byName[name]
	return method, ok
}

func (r *Registry) Lookup(name Name) (MethodMeta, bool) {
	method, ok := r.lookup(name)
	if !ok {
		return MethodMeta{}, false
	}
	return cloneMethodMeta(method.Meta), true
}

func (r *Registry) Metas() []MethodMeta {
	out := make([]MethodMeta, 0, len(r.names))
	for _, name := range r.names {
		out = append(out, cloneMethodMeta(r.byName[name].Meta))
	}
	return out
}

func cloneMethodMeta(meta MethodMeta) MethodMeta {
	meta.Errors = slices.Clone(meta.Errors)
	meta.CapabilityRules = cloneCapabilityRules(meta.CapabilityRules)
	meta.Acknowledgement = slices.Clone(meta.Acknowledgement)
	meta.Materializes = slices.Clone(meta.Materializes)
	return meta
}

func cloneCapabilityRules(rules []CapabilityRule) []CapabilityRule {
	out := slices.Clone(rules)
	for index := range out {
		out[index].When = slices.Clone(out[index].When)
		out[index].Requires = slices.Clone(out[index].Requires)
	}
	return out
}

func (r *Registry) StreamMethods() []string {
	var out []string
	for _, name := range r.names {
		if r.byName[name].Meta.Kind == KindStream {
			out = append(out, name.String())
		}
	}
	return out
}

func (r *Registry) query[Capability, Params, Response any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Response, error),
) {
	meta.Kind = kindUnary
	meta.Operation = OperationQuery
	meta.Idempotency = idempotencyNone
	meta.ReplayCursor = replayCursorNone
	r.registerUnary(meta, call)
}

func (r *Registry) command[Capability, Params, Response any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Response, error),
) *Method {
	meta.Kind = kindUnary
	meta.Operation = OperationCommand
	if meta.Idempotency == "" {
		meta.Idempotency = idempotencyReplayResponse
	}
	meta.ReplayCursor = replayCursorNone
	return r.registerUnary(meta, call)
}

func (r *Registry) extension[Capability, Params, Response any](meta MethodMeta, classify func(Params) (OperationKind, error), prepare func(Capability, context.Context, Params) (OperationKind, func(context.Context) (Response, error), error)) {
	meta.Kind = kindUnary
	meta.Result = reflect.TypeFor[Response]()
	r.registerExtension(meta, classify, func(target Capability, ctx context.Context, request Params) (OperationKind, func(context.Context) rawResult, error) {
		kind, call, err := prepare(target, ctx, request)
		if err != nil || call == nil {
			return kind, nil, err
		}
		return kind, func(ctx context.Context) rawResult {
			value, err := call(ctx)
			return rawResult{value: value, err: err}
		}, nil
	})
}

func (r *Registry) streamExtension[Capability, Params, Ack, Event any](meta MethodMeta, classify func(Params) (OperationKind, error), prepare func(Capability, context.Context, Params) (OperationKind, func(context.Context) (Ack, iter.Seq2[Event, error], error), error)) {
	meta.Kind = KindStream
	meta.Result = reflect.TypeFor[Ack]()
	meta.Event = reflect.TypeFor[Event]()
	r.registerExtension(meta, classify, func(target Capability, ctx context.Context, request Params) (OperationKind, func(context.Context) rawResult, error) {
		kind, call, err := prepare(target, ctx, request)
		if err != nil || call == nil {
			return kind, nil, err
		}
		return kind, func(ctx context.Context) rawResult {
			ack, events, err := call(ctx)
			return rawResult{value: ack, events: eraseEventType(events), err: err}
		}, nil
	})
}

func (r *Registry) registerExtension[Capability, Params any](meta MethodMeta, classify func(Params) (OperationKind, error), prepare func(Capability, context.Context, Params) (OperationKind, func(context.Context) rawResult, error)) {
	meta.Operation = OperationExtension
	meta.Idempotency = idempotencyOwnerResponse
	meta.ReplayCursor = replayCursorNone
	meta.Pagination = paginationNone
	meta.Params = reflect.TypeFor[Params]()
	r.add(meta, nil)
	registered := r.byName[meta.Name]
	registered.classify = func(parameters any) (OperationKind, error) {
		request, ok := parameters.(Params)
		if !ok {
			return "", fmt.Errorf("operation: %s received invalid parameters", meta.Name)
		}
		return classify(request)
	}
	registered.prepare = func(target any, ctx context.Context, parameters any) (*Method, error) {
		request, ok := parameters.(Params)
		if !ok {
			return nil, fmt.Errorf("operation: %s received invalid parameters", meta.Name)
		}
		capability, ok := target.(Capability)
		if !ok || !capabilityAvailable(capability) {
			return nil, fmt.Errorf("operation: target cannot handle %s", meta.Name)
		}
		kind, call, err := prepare(capability, ctx, request)
		if err != nil {
			return nil, err
		}
		expected, err := classify(request)
		if err != nil {
			return nil, err
		}
		if kind != expected {
			return nil, fmt.Errorf("operation: %s resolved inconsistent semantics", meta.Name)
		}
		resolved := cloneMethodMeta(registered.Meta)
		resolved.Operation = kind
		switch kind {
		case OperationQuery:
			resolved.Idempotency = idempotencyNone
		case OperationCommand:
			resolved.Idempotency = idempotencyOwnerResponse
		default:
			return nil, fmt.Errorf("operation: %s resolved an invalid operation kind", meta.Name)
		}
		if call == nil {
			return nil, fmt.Errorf("operation: %s resolved no invocation", meta.Name)
		}
		return &Method{Meta: resolved, invoke: func(_ any, ctx context.Context, _ any) rawResult {
			return call(ctx)
		}}, nil
	}
}

func (r *Registry) registerUnary[Capability, Params, Response any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Response, error),
) *Method {
	meta.Params = reflect.TypeFor[Params]()
	meta.Result = reflect.TypeFor[Response]()
	pagination, err := paginationOf(meta.Params, meta.Result)
	if err != nil {
		panic(fmt.Sprintf("operation: %s has invalid pagination shapes: %v", meta.Name, err))
	}
	meta.Pagination = pagination
	return r.add(meta, func(target any, ctx context.Context, parameters any) rawResult {
		typed, ok := parameters.(Params)
		if !ok {
			return rawResult{err: fmt.Errorf("operation: %s received parameters of type %T, want %s", meta.Name, parameters, meta.Params)}
		}
		capability, ok := target.(Capability)
		if !ok || !capabilityAvailable(capability) {
			return rawResult{err: fmt.Errorf("operation: target cannot handle %s", meta.Name)}
		}
		value, err := call(capability, ctx, typed)
		return rawResult{value: value, err: err}
	})
}

func (r *Registry) commandAck[Capability, Params any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) error,
) *Method {
	meta.Kind = kindUnary
	meta.Operation = OperationCommand
	if meta.Idempotency == "" {
		meta.Idempotency = idempotencyReplayResponse
	}
	meta.ReplayCursor = replayCursorNone
	meta.Pagination = paginationNone
	meta.Params = reflect.TypeFor[Params]()
	return r.add(meta, func(target any, ctx context.Context, parameters any) rawResult {
		typed, ok := parameters.(Params)
		if !ok {
			return rawResult{err: fmt.Errorf("operation: %s received parameters of type %T, want %s", meta.Name, parameters, meta.Params)}
		}
		capability, ok := target.(Capability)
		if !ok || !capabilityAvailable(capability) {
			return rawResult{err: fmt.Errorf("operation: target cannot handle %s", meta.Name)}
		}
		return rawResult{value: struct{}{}, err: call(capability, ctx, typed)}
	})
}

func (r *Registry) subscription[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq2[Event, error], error),
) {
	meta.ReplayCursor = replayCursorNone
	r.registerSubscription(meta, call)
}

func (r *Registry) registerSubscription[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq2[Event, error], error),
) {
	meta.Kind = KindStream
	meta.Operation = OperationSubscription
	meta.Idempotency = idempotencyNone
	r.registerStream(meta, call)
}

func (r *Registry) runSubscription[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq2[Event, error], error),
) {
	meta.ReplayCursor = ReplayCursorRun
	r.registerSubscription(meta, call)
}

func (r *Registry) runStreamOwnerCommand[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq2[Event, error], error),
) {
	meta.Kind = KindStream
	meta.Operation = OperationCommand
	meta.Idempotency = IdempotencyOwnerRun
	meta.ReplayCursor = ReplayCursorRun
	r.registerStream(meta, call)
}

func (r *Registry) registerStream[Capability, Params, Ack, Event any](
	meta MethodMeta,
	call func(Capability, context.Context, Params) (Ack, iter.Seq2[Event, error], error),
) {
	meta.Params = reflect.TypeFor[Params]()
	meta.Result = reflect.TypeFor[Ack]()
	meta.Event = reflect.TypeFor[Event]()
	meta.Pagination = paginationNone
	r.add(meta, func(target any, ctx context.Context, parameters any) rawResult {
		typed, ok := parameters.(Params)
		if !ok {
			return rawResult{err: fmt.Errorf("operation: %s received parameters of type %T, want %s", meta.Name, parameters, meta.Params)}
		}
		capability, ok := target.(Capability)
		if !ok || !capabilityAvailable(capability) {
			return rawResult{err: fmt.Errorf("operation: target cannot handle %s", meta.Name)}
		}
		ack, events, err := call(capability, ctx, typed)
		return rawResult{value: ack, events: eraseEventType(events), err: err}
	})
}

func capabilityAvailable(capability any) bool {
	return !lo.IsNil(capability)
}

func eraseEventType[Event any](events iter.Seq2[Event, error]) iter.Seq2[any, error] {
	if events == nil {
		return nil
	}
	return func(yield func(any, error) bool) {
		for event, err := range events {
			if err != nil {
				yield(nil, err)
				return
			}
			if !yield(event, nil) {
				return
			}
		}
	}
}

const (
	cursorField     = "cursor"
	limitField      = "limit"
	dataField       = "data"
	nextCursorField = "nextCursor"
)

func paginationOf(params, result reflect.Type) (PaginationKind, error) {
	if params == nil {
		return paginationNone, errors.New("params type is required")
	}
	if err := protocol.ValidateWireType(params); err != nil {
		return paginationNone, fmt.Errorf("invalid params declaration: %w", err)
	}
	params = protocol.WireBaseType(params)
	if params.Kind() != reflect.Struct {
		return paginationNone, fmt.Errorf("params must declare a JSON object, got %s", params)
	}
	page, err := isPageResult(result)
	if err != nil {
		return paginationNone, err
	}
	cursor, hasCursor := protocol.WireFieldNamed(params, cursorField)
	limit, hasLimit := protocol.WireFieldNamed(params, limitField)
	switch {
	case !page && hasCursor:
		return paginationNone, fmt.Errorf("%s has a cursor request field but its result is not a page", params)
	case !page:
		return paginationNone, nil
	case hasCursor != hasLimit:
		return paginationNone, fmt.Errorf("%s must declare cursor and limit together", params)
	case !hasCursor:
		return paginationNone, nil
	case cursor.Type.Kind() != reflect.String || !cursor.Optional:
		return paginationNone, fmt.Errorf("%s.cursor must be an optional string", params)
	case limit.Type.Kind() != reflect.Pointer || limit.Type.Elem().Kind() != reflect.Int || !limit.Optional:
		return paginationNone, fmt.Errorf("%s.limit must be a pointer-backed optional int", params)
	default:
		return paginationCursor, nil
	}
}

func isPageResult(result reflect.Type) (bool, error) {
	if result == nil {
		return false, nil
	}
	if err := protocol.ValidateWireType(result); err != nil {
		return false, fmt.Errorf("invalid result declaration: %w", err)
	}
	result = protocol.WireBaseType(result)
	data, hasData := protocol.WireFieldNamed(result, dataField)
	nextCursor, hasNextCursor := protocol.WireFieldNamed(result, nextCursorField)
	switch {
	case !hasData && !hasNextCursor:
		return false, nil
	case !hasData || !hasNextCursor:
		return false, fmt.Errorf("page results require data and nextCursor together; result=%s", result)
	case data.Type.Kind() != reflect.Slice || data.Optional:
		return false, fmt.Errorf("%s.data must be a required array", result)
	case nextCursor.Type.Kind() != reflect.String || !nextCursor.Optional:
		return false, fmt.Errorf("%s.nextCursor must be an optional string", result)
	default:
		return true, nil
	}
}

func Contract() *Registry { return contract }
