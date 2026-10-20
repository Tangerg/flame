package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"sync"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type CallComposition struct {
	calls     *CommandCalls
	callbacks *pluginsdk.Callbacks
	admission *plugins.Admission
	mu        sync.Mutex
	captures  map[*modelCapture]struct{}
}

func NewCallComposition(calls *CommandCalls, callbacks *pluginsdk.Callbacks, admission *plugins.Admission) (*CallComposition, error) {
	if calls == nil || callbacks == nil || admission == nil {
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
	if location != binding {
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

func (o *observedModel) prepare(ctx context.Context, request *chat.Request, streaming bool) (*sdk.ClientSession, *pluginsdk.Binding, error) {
	if err := request.Validate(); err != nil {
		return nil, nil, err
	}
	_, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return nil, nil, err
	}
	method, err := pluginsdk.ModelMethod()
	if err != nil {
		return nil, nil, err
	}
	name := method.Descriptor().Name
	if streaming {
		stream, err := pluginsdk.ModelStreamMethod()
		if err != nil {
			return nil, nil, err
		}
		name = stream.Descriptor().Name
	}
	r := o.binding
	if err := o.owner.admitBinding(ctx, r); err != nil {
		return nil, nil, err
	}
	session, err := o.owner.calls.pool.ServiceSession(ctx, r.ServerID)
	if err != nil {
		return nil, nil, err
	}
	if _, err := o.owner.calls.pool.SessionActor(ctx, r.ServerID, session); err != nil {
		return nil, nil, err
	}
	exported, err := pluginsdk.NewCapabilityBinding(session, r.Export, contract)
	if err != nil {
		return nil, nil, err
	}
	bound, err := exported.Method(name)
	return session, bound, err
}

// observationAdmission is this transparent contract's one-delivery rule, not a
// universal restriction on decoration. The captured request cannot be transformed.
type observationAdmission struct {
	mu           sync.Mutex
	request      *chat.Request
	id           string
	entered      bool
	known        *chat.Response
	pending      *chat.ResponseDelta
	forwarded    chan struct{}
	violation    error
	innerFailure error
}

func (a *observationAdmission) enter(command pluginsdk.Command[chat.Request]) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	exact, err := sameObservationJSON(a.request, &command.Input)
	if a.entered || command.ID != a.id || err != nil || !exact {
		a.violation = &capability.Refusal{Code: "observer_contract", Message: "transparent observer cannot repeat or change its continuation"}
		a.violation = errors.Join(a.violation, err)
		return a.violation
	}
	a.entered = true
	return nil
}

func (a *observationAdmission) retain(response *chat.Response, failure error) {
	a.mu.Lock()
	a.innerFailure = failure
	if response != nil && response.Validate() == nil {
		a.known = response.Clone()
	}
	a.mu.Unlock()
}

func (a *observationAdmission) outcome(response *chat.Response, err error) (*chat.Response, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	err = errors.Join(err, a.violation, a.innerFailure)
	if err != nil {
		return a.known.Clone(), err
	}
	exact, encodingErr := sameObservationJSON(a.known, response)
	if !a.entered || a.known == nil || a.pending != nil || encodingErr != nil || !exact {
		return a.known.Clone(), errors.Join(encodingErr, errors.New("model observer changed or omitted its captured outcome"))
	}
	return a.known.Clone(), nil
}

func sameObservationJSON(left, right any) (bool, error) {
	encodedLeft, err := jsoninput.Encode(left)
	if err != nil {
		return false, err
	}
	encodedRight, err := jsoninput.Encode(right)
	if err != nil {
		return false, err
	}
	return encodedLeft == encodedRight, nil
}

func (o *observedModel) Call(ctx context.Context, request *chat.Request) (*chat.Response, error) {
	ctx, err := enterContributionPhase(ctx, o.binding)
	if err != nil {
		return nil, err
	}
	session, binding, err := o.prepare(ctx, request, false)
	if err != nil {
		return nil, err
	}
	invocation, _ := pluginsdk.InvocationFromContext(ctx)
	admission := &observationAdmission{request: request.Clone(), id: invocation.ID}
	next, err := pluginsdk.NewModelCallback(func(requestCtx context.Context, command pluginsdk.Command[chat.Request]) (*chat.Response, error) {
		if err := admission.enter(command); err != nil {
			return nil, err
		}
		calling, release := modelContinuationContext(ctx, requestCtx)
		defer release()
		if err := calling.Err(); err != nil {
			return nil, errors.Join(err, context.Cause(calling))
		}
		response, err := o.next.Call(calling, command.Input.Clone())
		admission.retain(response, err)
		return response, err
	})
	if err != nil {
		return nil, err
	}
	metadata, release, err := o.owner.calls.pool.OpenRPC(ctx, o.binding.ServerID, session, next)
	if err != nil {
		return nil, err
	}
	defer release()
	input, err := jsoninput.Encode(admission.request)
	if err != nil {
		return nil, err
	}
	raw, err := binding.InvokeWithMetadata(ctx, invocation.ID, input, metadata)
	release()
	if err != nil {
		return admission.outcome(nil, err)
	}
	var result pluginsdk.ModelResult
	if err := json.Unmarshal(raw.JSON(), &result, json.RejectUnknownMembers(true)); err != nil {
		return admission.outcome(nil, err)
	}
	if result.Failure != nil {
		err = result.Failure
	}
	return admission.outcome(result.Response, err)
}

func (o *streamedObservedModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	frozen := request.Clone()
	return func(yield func(*chat.ResponseDelta, error) bool) {
		calling, err := enterContributionPhase(ctx, o.binding)
		if err != nil {
			yield(nil, err)
			return
		}
		session, binding, err := o.prepare(calling, frozen, true)
		if err != nil {
			yield(nil, err)
			return
		}
		invocation, _ := pluginsdk.InvocationFromContext(ctx)
		admission := &observationAdmission{request: frozen, id: invocation.ID}
		next, err := pluginsdk.NewModelStreamCallback(func(requestCtx context.Context, command pluginsdk.Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
			return func(yield func(*chat.ResponseDelta, error) bool) {
				if err := admission.enter(command); err != nil {
					yield(nil, err)
					return
				}
				continuation, release := modelContinuationContext(calling, requestCtx)
				defer release()
				if err := continuation.Err(); err != nil {
					yield(nil, errors.Join(err, context.Cause(continuation)))
					return
				}
				var accumulated chat.ResponseAccumulator
				sequence := o.nextStream.Stream(continuation, command.Input.Clone())
				if sequence == nil {
					yield(nil, errors.New("model observer continuation returned no stream"))
					return
				}
				for delta, err := range sequence {
					if err != nil {
						if response, complete := accumulated.Response(); complete == nil {
							admission.retain(response, err)
						} else {
							admission.retain(nil, err)
						}
						yield(nil, err)
						return
					}
					if err := accumulated.Add(delta); err != nil {
						yield(nil, err)
						return
					}
					admission.mu.Lock()
					pending := admission.pending != nil
					if !pending {
						admission.pending = delta.Clone()
						admission.forwarded = make(chan struct{})
					}
					forwarded := admission.forwarded
					admission.mu.Unlock()
					if pending {
						yield(nil, errors.New("model observer did not forward its pending chunk"))
						return
					}
					if !yield(delta.Clone(), nil) {
						return
					}
					// Callback frame writing alone does not mean the remote
					// observer has forwarded it to this chain's consumer.
					select {
					case <-forwarded:
					case <-continuation.Done():
						return
					}
				}
				if response, err := accumulated.Response(); err == nil {
					admission.retain(response, nil)
				}
			}
		})
		if err != nil {
			yield(nil, err)
			return
		}
		metadata, release, err := o.owner.calls.pool.OpenRPC(calling, o.binding.ServerID, session, next)
		if err != nil {
			yield(nil, err)
			return
		}
		defer release()
		input, err := jsoninput.Encode(frozen)
		if err != nil {
			yield(nil, err)
			return
		}
		var result *pluginsdk.ModelResult
		for value, err := range o.owner.calls.pool.Stream(calling, o.binding.ServerID, session, binding, invocation.ID, input, metadata) {
			if err != nil {
				yield(nil, err)
				return
			}
			if value.Chunk != nil {
				var delta chat.ResponseDelta
				if err := json.Unmarshal(*value.Chunk, &delta, json.RejectUnknownMembers(true)); err != nil {
					yield(nil, err)
					return
				}
				admission.mu.Lock()
				exact, comparisonErr := sameObservationJSON(admission.pending, &delta)
				admission.mu.Unlock()
				if comparisonErr != nil || !exact {
					yield(nil, errors.Join(comparisonErr, errors.New("model observer changed or reordered its captured chunk")))
					return
				}
				if !yield(delta.Clone(), nil) {
					return
				}
				admission.mu.Lock()
				admission.pending = nil
				close(admission.forwarded)
				admission.mu.Unlock()
			} else if value.Result != nil {
				result = new(pluginsdk.ModelResult)
				if err := json.Unmarshal(*value.Result, result, json.RejectUnknownMembers(true)); err != nil {
					yield(nil, err)
					return
				}
			}
		}
		if result == nil {
			yield(nil, pluginsdk.ErrStreamUnconfirmed)
			return
		}
		if result.Failure != nil {
			err = result.Failure
		}
		if _, err := admission.outcome(result.Response, err); err != nil {
			yield(nil, err)
		}
	}
}
