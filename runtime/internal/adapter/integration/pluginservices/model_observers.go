package pluginservices

import (
	"context"
	"errors"
	"iter"
	"sync"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/scopewire"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	framehost "github.com/Tangerg/frame/host"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
)

type CallComposition struct {
	calls     *CommandCalls
	callbacks *pluginmcp.Callbacks
	admission framehost.Admission
	mu        sync.Mutex
	captures  map[*modelCapture]struct{}
}

func NewCallComposition(calls *CommandCalls, callbacks *pluginmcp.Callbacks, admission framehost.Admission) (*CallComposition, error) {
	if calls == nil || callbacks == nil || !admission.Valid() {
		return nil, errors.New("model composition: installed calls and callbacks are required")
	}
	return &CallComposition{calls: calls, callbacks: callbacks, admission: admission, captures: make(map[*modelCapture]struct{})}, nil
}

func (o *CallComposition) Resolve(ctx context.Context) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return nil, err
	}
	return plugins.ResolveComposition(ctx, o.calls.installations, o.calls.compositions, contract)
}

func (o *CallComposition) Validate(ctx context.Context, bindings []plugins.ContributionBinding) error {
	for _, binding := range bindings {
		if err := o.admitBinding(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func (o *CallComposition) admitBinding(ctx context.Context, binding plugins.ContributionBinding) error {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return err
	}
	location, err := o.calls.resolveContribution(ctx, plugins.CallRequest{Installation: binding.InstallationID, Digest: binding.Digest, Contribution: binding.ContributionID, Contract: contract})
	if err != nil {
		return err
	}
	if !location.SameContribution(binding) {
		return plugin.ErrStale
	}
	return nil
}

func (o *CallComposition) Bind(model chat.Model, streamer chat.Streamer, bindings []plugins.ContributionBinding) (chat.Model, chat.Streamer) {
	for index := len(bindings) - 1; index >= 0; index-- {
		layer := &observedModel{owner: o, binding: bindings[index], next: model, nextStream: streamer}
		model = layer
		if streamer != nil {
			streaming := &streamedObservedModel{observedModel: layer}
			model, streamer = streaming, streaming
		}
	}
	return model, streamer
}

type observedModel struct {
	owner      *CallComposition
	binding    plugins.ContributionBinding
	next       chat.Model
	nextStream chat.Streamer
}

type streamedObservedModel struct{ *observedModel }

func (o *observedModel) prepare(ctx context.Context, request *chat.Request, delivery callDelivery) (*mcp.BorrowedSession, *pluginmcp.Binding, error) {
	if err := request.Validate(); err != nil {
		return nil, nil, err
	}
	_, err := pluginmcp.InvocationFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return nil, nil, err
	}
	method, err := pluginsdk.ModelObserverCallMethod()
	if err != nil {
		return nil, nil, err
	}
	name := method.Descriptor().Name
	if delivery == streamedCall {
		stream, err := pluginsdk.ModelObserverStreamMethod()
		if err != nil {
			return nil, nil, err
		}
		name = stream.Descriptor().Name
	}
	r := o.binding
	if err := o.owner.admitBinding(ctx, r); err != nil {
		return nil, nil, err
	}
	borrowed, err := borrowContribution(ctx, o.owner.calls.pool, r)
	if err != nil {
		return nil, nil, err
	}
	transferred := false
	defer func() {
		if !transferred {
			borrowed.Close()
		}
	}()
	ctx = borrowed.Context()
	session := borrowed.Session()
	if _, err := o.owner.calls.pool.SessionActor(ctx, r.ServerID, session); err != nil {
		return nil, nil, err
	}
	exported, err := pluginmcp.NewCapabilityBinding(session, r.Export, contract)
	if err != nil {
		return nil, nil, err
	}
	bound, err := exported.Method(name)
	if err != nil {
		return nil, nil, err
	}
	transferred = true
	return borrowed, bound, nil
}

func (o *observedModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	ctx, err := enterContributionPhase(ctx, o.binding)
	if err != nil {
		return nil, continuationNotExecuted(err)
	}
	borrowed, binding, err := o.prepare(ctx, request, unaryCall)
	if err != nil {
		return nil, continuationNotExecuted(err)
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	session := borrowed.Session()
	invocation, _ := pluginmcp.InvocationFromContext(ctx)
	frozen := request.Clone()
	input, err := framevalue.EncodeJSON((*scopewire.Request)(frozen))
	if err != nil {
		return nil, continuationNotExecuted(err)
	}
	admission := &observedContinuation{request: input, id: pluginmcp.CommandID(invocation.ID)}
	next, err := pluginsdk.NewModelCallback(func(requestCtx context.Context, command pluginmcp.Command[scopewire.Request]) (*chat.Response, error) {
		if err := admission.enter(command); err != nil {
			return nil, err
		}
		calling, release := modelContinuationContext(ctx, requestCtx)
		defer release()
		if err := calling.Err(); err != nil {
			failure := continuationNotExecuted(errors.Join(err, context.Cause(calling)))
			admission.finish(nil, failure)
			return nil, failure
		}
		response, err := o.next.Call(calling, frozen.Clone())
		admission.finish(response, err)
		return response, err
	})
	if err != nil {
		return nil, continuationNotExecuted(err)
	}
	metadata, release, err := o.owner.calls.pool.OpenRPC(ctx, o.binding.ServerID, session, next)
	if err != nil {
		return nil, continuationNotExecuted(err)
	}
	defer release()
	_, observerErr := binding.InvokeWithMetadata(ctx, admission.id, input, metadata)
	release()
	return admission.outcome(observerErr)
}

func (o *streamedObservedModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	frozen := request.Clone()
	return func(yield func(*chat.ResponseDelta, error) bool) {
		calling, err := enterContributionPhase(ctx, o.binding)
		if err != nil {
			yield(nil, continuationNotExecuted(err))
			return
		}
		borrowed, binding, err := o.prepare(calling, frozen, streamedCall)
		if err != nil {
			yield(nil, continuationNotExecuted(err))
			return
		}
		defer borrowed.Close()
		calling = borrowed.Context()
		session := borrowed.Session()
		invocation, _ := pluginmcp.InvocationFromContext(calling)
		input, err := framevalue.EncodeJSON((*scopewire.Request)(frozen))
		if err != nil {
			yield(nil, continuationNotExecuted(err))
			return
		}
		admission := &observedContinuation{request: input, id: pluginmcp.CommandID(invocation.ID)}
		next, err := pluginsdk.NewModelStreamCallback(func(requestCtx context.Context, command pluginmcp.Command[scopewire.Request]) iter.Seq2[*chat.ResponseDelta, error] {
			return func(yield func(*chat.ResponseDelta, error) bool) {
				if err := admission.enter(command); err != nil {
					yield(nil, err)
					return
				}
				continuation, release := modelContinuationContext(calling, requestCtx)
				defer release()
				if err := continuation.Err(); err != nil {
					failure := continuationNotExecuted(errors.Join(err, context.Cause(continuation)))
					admission.finish(nil, failure)
					yield(nil, failure)
					return
				}
				admission.relay(continuation, o.nextStream.Stream(continuation, frozen.Clone()), yield)
			}
		})
		if err != nil {
			yield(nil, continuationNotExecuted(err))
			return
		}
		metadata, release, err := o.owner.calls.pool.OpenRPC(calling, o.binding.ServerID, session, next)
		if err != nil {
			yield(nil, continuationNotExecuted(err))
			return
		}
		defer release()
		var observerErr error
		for value, err := range o.owner.calls.pool.Stream(calling, o.binding.ServerID, session, binding, invocation.ID, input, metadata) {
			if err != nil {
				observerErr = err
				break
			}
			if value.Chunk == nil {
				continue
			}
			chunk, err := admission.matchChunk(*value.Chunk)
			if err != nil {
				observerErr = err
				break
			}
			if !yield(chunk.delta.Clone(), nil) {
				return
			}
			admission.acknowledgeChunk(chunk)
		}
		release()
		if _, err := admission.outcome(observerErr); err != nil {
			yield(nil, err)
		}
	}
}
