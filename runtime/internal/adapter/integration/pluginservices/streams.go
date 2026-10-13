package pluginservices

import (
	"context"
	"errors"
	"iter"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type installedStream struct{ installedCall }

func (c *CommandCalls) PrepareStream(_ context.Context, request plugins.CallRequest) (plugins.StreamCall, error) {
	method, err := admitCallRequest(request, true)
	if err != nil {
		return nil, err
	}
	if method.Kind() != capability.Mutation {
		return nil, plugin.ErrInvalid
	}
	return &installedStream{installedCall: installedCall{calls: c, request: request, method: method}}, nil
}

func (c *installedStream) Stream(ctx context.Context) iter.Seq2[plugins.StreamEvent, error] {
	return func(yield func(plugins.StreamEvent, error) bool) {
		fail := func(err error) { yield(plugins.StreamEvent{}, errors.Join(plugin.ErrCommandUnconfirmed, err)) }
		record, err := c.calls.admitCommandRoute(ctx, c.request.Method, c.request.Input, c.accepts, c.resolve, func(plugins.ContributionBinding) capability.Target { return c.request.Target }, true)
		if err != nil {
			yield(plugins.StreamEvent{}, err)
			return
		}
		ctx, err = pluginsdk.WithInvocation(ctx, record.Invocation)
		if err != nil {
			yield(plugins.StreamEvent{}, err)
			return
		}
		if record.Phase != plugin.CommandPrepared {
			defer c.calls.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
		}
		location, err := c.calls.original(ctx, record)
		if err != nil {
			fail(err)
			return
		}
		session, err := c.calls.pool.ServiceSession(ctx, location.ServerID)
		if err != nil {
			fail(err)
			return
		}
		contract, err := capability.Parse(record.Contract.JSON())
		if err != nil {
			fail(err)
			return
		}
		exported, err := pluginsdk.NewCapabilityBinding(session, record.Export, contract)
		if err != nil {
			fail(err)
			return
		}
		binding, err := exported.Method(record.Method)
		if err != nil {
			fail(err)
			return
		}
		var result jsoninput.Input
		if record.Phase == plugin.CommandPrepared {
			metadata, release, err := c.calls.pool.OpenRPC(ctx, location.ServerID, session)
			if err != nil {
				yield(plugins.StreamEvent{}, err)
				return
			}
			defer release()

			if err := c.calls.routing.Dispatch(ctx, record.ID, func(ctx context.Context, record plugin.CommandBinding) error {
				_, err := c.calls.original(ctx, record)
				return err
			}); err != nil {
				yield(plugins.StreamEvent{}, err)
				return
			}
			defer c.calls.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
			for value, err := range c.calls.pool.Stream(ctx, location.ServerID, session, binding, record.ID, record.Input, metadata) {
				if err != nil {
					if refusal, known := errors.AsType[*capability.Refusal](err); known {
						if observeErr := c.calls.routing.Observe(ctx, record.ID); observeErr != nil {
							fail(errors.Join(err, observeErr))
							return
						}
						yield(plugins.StreamEvent{}, refusal)
						return
					}
					fail(err)
					return
				}
				if value.Chunk != nil {
					chunk, err := jsoninput.Parse(*value.Chunk)
					if err != nil {
						fail(err)
						return
					}
					if !yield(plugins.StreamEvent{Kind: plugins.StreamChunk, Value: chunk}, nil) {
						return
					}
				} else if value.Result != nil {
					result, err = jsoninput.Parse(*value.Result)
					if err != nil {
						fail(err)
						return
					}
				}
			}
		} else {
			var found bool
			result, found, err = binding.ReadReceipt(ctx, record.ID, record.Input)
			if err != nil {
				if _, known := errors.AsType[*capability.Refusal](err); known {
					if observeErr := c.calls.routing.Observe(ctx, record.ID); observeErr != nil {
						fail(errors.Join(err, observeErr))
						return
					}
					yield(plugins.StreamEvent{}, err)
					return
				}
				fail(err)
				return
			}
			if !found {
				fail(plugin.ErrCommandUnconfirmed)
				return
			}
		}
		if err := result.Validate(); err != nil {
			fail(err)
			return
		}
		if err := c.calls.routing.Observe(ctx, record.ID); err != nil {
			fail(err)
			return
		}
		yield(plugins.StreamEvent{Kind: plugins.StreamResult, Value: result}, nil)
	}
}
