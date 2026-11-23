package pluginservices

import (
	"context"
	"encoding/json/jsontext"
	"iter"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
)

type installedStream struct{ installedCall }

func (c *CommandCalls) PrepareStream(_ context.Context, request plugins.CallRequest) (plugins.StreamCall, error) {
	method, err := admitCallRequest(request, streamedCall)
	if err != nil {
		return nil, err
	}
	return &installedStream{installedCall: installedCall{calls: c, request: request, method: method}}, nil
}

func (c *installedStream) Stream(ctx context.Context) iter.Seq2[plugins.StreamEvent, error] {
	return func(yield func(plugins.StreamEvent, error) bool) {
		stream := c.streamCommand
		if c.Kind() == frame.Query {
			stream = c.streamQuery
		}
		if err := stream(ctx, yield); err != nil {
			yield(plugins.StreamEvent{}, err)
		}
	}
}

func (c *installedStream) streamCommand(ctx context.Context, yield func(plugins.StreamEvent, error) bool) error {
	record, err := c.calls.admitCommandRoute(ctx, c.route(), streamedCall)
	if err != nil {
		return err
	}
	ctx, command, err := c.calls.openCommand(ctx, record)
	if err != nil {
		return err
	}
	defer command.close()
	settle := func(result framevalue.JSON, err error) error {
		result, err = command.settle(ctx, result, err, framevalue.JSON.Validate)
		if err != nil {
			return err
		}
		yield(plugins.StreamEvent{Kind: plugins.StreamResult, Value: result}, nil)
		return nil
	}
	if record.Phase != plugin.CommandPrepared {
		return settle(command.readReceipt(ctx))
	}
	metadata, release, err := c.calls.pool.OpenRPC(ctx, command.binding.ServerID, command.session)
	if err != nil {
		return err
	}
	defer release()
	if err := command.dispatch(ctx); err != nil {
		return err
	}
	var result *framevalue.JSON
	for value, err := range c.calls.pool.Stream(ctx, command.binding.ServerID, command.session, command.method, record.ID, record.Input, metadata) {
		event, err := streamEvent(value, err)
		if err != nil {
			return settle(framevalue.JSON{}, err)
		}
		if event.Kind == plugins.StreamResult {
			result = &event.Value
			continue
		}
		if !yield(event, nil) {
			return nil
		}
	}
	if result == nil {
		return settle(framevalue.JSON{}, pluginmcp.ErrStreamUnconfirmed)
	}
	return settle(*result, nil)
}

func (c *installedStream) streamQuery(ctx context.Context, yield func(plugins.StreamEvent, error) bool) error {
	ctx, query, err := c.openQuery(ctx)
	if err != nil {
		return err
	}
	defer query.release()
	for value, err := range c.calls.pool.Stream(ctx, query.location.ServerID, query.session, query.method, "", c.request.Input, query.metadata) {
		event, err := streamEvent(value, err)
		if err != nil {
			return err
		}
		if !yield(event, nil) {
			return nil
		}
	}
	return nil
}

func streamEvent(value pluginmcp.StreamValue[jsontext.Value, jsontext.Value], err error) (plugins.StreamEvent, error) {
	if err != nil {
		return plugins.StreamEvent{}, err
	}
	if chunk, ok := value.Chunk(); ok {
		parsed, err := framevalue.ParseJSON(chunk)
		return plugins.StreamEvent{Kind: plugins.StreamChunk, Value: parsed}, err
	}
	if result, ok := value.Result(); ok {
		parsed, err := framevalue.ParseJSON(result)
		return plugins.StreamEvent{Kind: plugins.StreamResult, Value: parsed}, err
	}
	return plugins.StreamEvent{}, pluginmcp.ErrStreamUnconfirmed
}
