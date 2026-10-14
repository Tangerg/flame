package pluginservices

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"iter"
	"sync"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type ModelComposition struct {
	calls     *CommandCalls
	callbacks *pluginsdk.Callbacks
	admission *plugins.Admission
	mu        sync.Mutex
	auxiliary map[*auxiliaryModelCapture]struct{}
}

func NewModelComposition(calls *CommandCalls, callbacks *pluginsdk.Callbacks, admission *plugins.Admission) (*ModelComposition, error) {
	if calls == nil || callbacks == nil || admission == nil {
		return nil, errors.New("model composition: installed calls and callbacks are required")
	}
	return &ModelComposition{calls: calls, callbacks: callbacks, admission: admission, auxiliary: make(map[*auxiliaryModelCapture]struct{})}, nil
}

func (o *ModelComposition) Resolve(ctx context.Context) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return nil, err
	}
	return plugins.ResolveComposition(ctx, o.calls.installations, o.calls.compositions, contract)
}

func (o *ModelComposition) Validate(ctx context.Context, bindings []plugins.ContributionBinding) error {
	for _, binding := range bindings {
		if err := o.admitBinding(ctx, binding); err != nil {
			return err
		}
	}
	return nil
}

func (o *ModelComposition) admitBinding(ctx context.Context, binding plugins.ContributionBinding) error {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return err
	}
	call := installedCall{calls: o.calls, request: plugins.CallRequest{Installation: binding.InstallationID, Digest: binding.Digest, Contribution: binding.ContributionID, Contract: contract}}
	location, err := call.resolve(ctx)
	if err != nil {
		return err
	}
	if location != binding {
		return plugin.ErrStale
	}
	return nil
}

func (o *ModelComposition) Bind(model chat.Model, streamer chat.Streamer, bindings []plugins.ContributionBinding) (chat.Model, chat.Streamer) {
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
	owner      *ModelComposition
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
	session, binding, err := o.prepare(ctx, request, false)
	if err != nil {
		return nil, err
	}
	invocation, _ := pluginsdk.InvocationFromContext(ctx)
	admission := &observationAdmission{request: request.Clone(), id: invocation.ID}
	next, err := pluginsdk.NewModelCallback(func(ctx context.Context, command pluginsdk.Command[chat.Request]) (*chat.Response, error) {
		if err := admission.enter(command); err != nil {
			return nil, err
		}
		response, err := o.next.Call(ctx, command.Input.Clone())
		admission.retain(response, err)
		return response, err
	})
	if err != nil {
		return nil, err
	}
	ref, release, err := o.owner.callbacks.Open(ctx, session, observerLimits(), next)
	if err != nil {
		return nil, err
	}
	defer release()
	metadata, err := pluginsdk.CallbackMetadata(ref)
	if err != nil {
		return nil, err
	}
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

func observerLimits() pluginsdk.CallbackLimits {
	return pluginsdk.CallbackLimits{MaxRequestBytes: 4 << 20, MaxResponseBytes: 4 << 20}
}

func (o *streamedObservedModel) Stream(ctx context.Context, request *chat.Request) iter.Seq2[*chat.ResponseDelta, error] {
	frozen := request.Clone()
	return func(yield func(*chat.ResponseDelta, error) bool) {
		session, binding, err := o.prepare(ctx, frozen, true)
		if err != nil {
			yield(nil, err)
			return
		}
		invocation, _ := pluginsdk.InvocationFromContext(ctx)
		admission := &observationAdmission{request: frozen, id: invocation.ID}
		next, err := pluginsdk.NewModelStreamCallback(func(ctx context.Context, command pluginsdk.Command[chat.Request]) iter.Seq2[*chat.ResponseDelta, error] {
			return func(yield func(*chat.ResponseDelta, error) bool) {
				if err := admission.enter(command); err != nil {
					yield(nil, err)
					return
				}
				var accumulated chat.ResponseAccumulator
				sequence := o.nextStream.Stream(ctx, command.Input.Clone())
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
					case <-ctx.Done():
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
		ref, release, err := o.owner.callbacks.Open(ctx, session, observerLimits(), next)
		if err != nil {
			yield(nil, err)
			return
		}
		defer release()
		metadata, err := pluginsdk.CallbackMetadata(ref)
		if err != nil {
			yield(nil, err)
			return
		}
		input, err := jsoninput.Encode(frozen)
		if err != nil {
			yield(nil, err)
			return
		}
		var result *pluginsdk.ModelResult
		for value, err := range o.owner.calls.pool.Stream(ctx, o.binding.ServerID, session, binding, invocation.ID, input, metadata) {
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
