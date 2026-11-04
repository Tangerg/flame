package pluginservices

import (
	"context"
	"errors"
	"iter"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

type installedStream struct{ installedCall }

func (c *CommandCalls) PrepareStream(_ context.Context, request plugins.CallRequest) (plugins.StreamCall, error) {
	method, err := admitCallRequest(request, true)
	if err != nil {
		return nil, err
	}
	return &installedStream{installedCall: installedCall{calls: c, request: request, method: method}}, nil
}

func (c *installedStream) Stream(ctx context.Context) iter.Seq2[plugins.StreamEvent, error] {
	return func(yield func(plugins.StreamEvent, error) bool) {
		if c.Kind() == portable.Query {
			c.streamQuery(ctx, yield)
			return
		}
		fail := func(err error) { yield(plugins.StreamEvent{}, errors.Join(plugin.ErrCommandUnconfirmed, err)) }
		record, err := c.calls.admitCommandRoute(ctx, c.request.Method, c.request.Input, c.accepts, c.resolve, func(plugins.ContributionBinding) portable.Target { return c.request.Target }, true)
		if err != nil {
			yield(plugins.StreamEvent{}, err)
			return
		}
		ctx, err = pluginmcp.WithInvocation(ctx, record.Invocation)
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
		borrowed, err := c.calls.pool.BorrowActivation(ctx, location.ServerID, record.Activation)
		if err != nil {
			fail(err)
			return
		}
		defer borrowed.Close()
		ctx = borrowed.Context()
		session := borrowed.Session()
		contract, err := plugincontract.ParseCapability(record.Contract.Bytes())
		if err != nil {
			fail(err)
			return
		}
		exported, err := pluginmcp.NewCapabilityBinding(session, record.Export, contract)
		if err != nil {
			fail(err)
			return
		}
		binding, err := exported.Method(record.Method)
		if err != nil {
			fail(err)
			return
		}
		var result framevalue.JSON
		if record.Phase == plugin.CommandPrepared {
			metadata, release, err := c.calls.pool.OpenRPC(ctx, location.ServerID, session)
			if err != nil {
				yield(plugins.StreamEvent{}, err)
				return
			}
			defer release()

			if err := c.calls.routing.Dispatch(ctx, record.ID, c.calls.admitDispatch); err != nil {
				yield(plugins.StreamEvent{}, err)
				return
			}
			defer c.calls.publish.Notify(invalidation.Notice{Resource: invalidation.PluginData})
			for value, err := range c.calls.pool.Stream(ctx, location.ServerID, session, binding, record.ID, record.Input, metadata) {
				if err != nil {
					if refusal, known := errors.AsType[*framemcp.Refusal](err); known {
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
					chunk, err := framevalue.ParseJSON(*value.Chunk)
					if err != nil {
						fail(err)
						return
					}
					if !yield(plugins.StreamEvent{Kind: plugins.StreamChunk, Value: chunk}, nil) {
						return
					}
				} else if value.Result != nil {
					result, err = framevalue.ParseJSON(*value.Result)
					if err != nil {
						fail(err)
						return
					}
				}
			}
		} else {
			var found bool
			result, found, err = binding.ReadReceipt(ctx, pluginmcp.CommandID(record.ID), record.Input)
			if err != nil {
				if _, known := errors.AsType[*framemcp.Refusal](err); known {
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

func (c *installedStream) streamQuery(ctx context.Context, yield func(plugins.StreamEvent, error) bool) {
	fail := func(err error) { yield(plugins.StreamEvent{}, err) }
	location, err := c.resolve(ctx)
	if err != nil {
		fail(err)
		return
	}
	borrowed, err := c.calls.pool.BorrowSession(ctx, location.ServerID)
	if err != nil {
		fail(err)
		return
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	session := borrowed.Session()
	exported, err := pluginmcp.ResolveCapability(ctx, session, location.Export, c.request.Contract)
	if err != nil {
		fail(err)
		return
	}
	binding, err := exported.Method(c.request.Method)
	if err != nil {
		fail(err)
		return
	}
	if err := validateInput(ctx, binding, c.request.Input); err != nil {
		fail(err)
		return
	}
	invocation, err := operation.Capture(ctx, c.request.Target)
	if err != nil {
		fail(err)
		return
	}
	ctx, err = pluginmcp.WithInvocation(ctx, invocation)
	if err != nil {
		fail(err)
		return
	}
	metadata, release, err := c.calls.pool.OpenRPC(ctx, location.ServerID, session)
	if err != nil {
		fail(err)
		return
	}
	defer release()
	for value, err := range c.calls.pool.Stream(ctx, location.ServerID, session, binding, "", c.request.Input, metadata) {
		if err != nil {
			fail(err)
			return
		}
		var event plugins.StreamEvent
		if value.Chunk != nil {
			event.Kind = plugins.StreamChunk
			event.Value, err = framevalue.ParseJSON(*value.Chunk)
		} else if value.Result != nil {
			event.Kind = plugins.StreamResult
			event.Value, err = framevalue.ParseJSON(*value.Result)
		} else {
			err = pluginmcp.ErrStreamUnconfirmed
		}
		if err != nil {
			fail(err)
			return
		}
		if !yield(event, nil) {
			return
		}
	}
}
