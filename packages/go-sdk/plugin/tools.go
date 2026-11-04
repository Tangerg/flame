package plugin

import (
	"context"
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	framevalue "github.com/Tangerg/frame/value"

	sdk "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const ToolCommandPrefix = "tool:"

// RegisterTools retains Scope's frozen Tool contracts and failure semantics.
// MCP metadata carries Runtime's logical identity; model arguments never own it.
func RegisterTools(server *sdk.Server, tools ...tool.Tool) error {
	if server == nil {
		return errors.New("plugin: server is required")
	}
	bindings := make([]tool.Binding, len(tools))
	for index, executable := range tools {
		binding, err := tool.Bind(executable)
		if err != nil {
			return err
		}
		bindings[index] = binding
	}
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
			if call, ok := request.(*sdk.CallToolRequest); ok && call.Params != nil {
				arguments := call.Params.Arguments
				if len(arguments) == 0 {
					arguments = []byte("{}")
				}
				input, err := framevalue.ParseJSON(arguments)
				if err != nil {
					return nil, err
				}
				ctx, err = pluginmcp.AdmitRequest(ctx, input, call.Params.Meta)
				if err != nil {
					return nil, err
				}
				if _, attached := call.Params.Meta[ToolContextMetadataKey]; attached {
					captured, err := ToolContextFromMetadata(call.Params.Meta)
					if err != nil {
						return nil, err
					}
					ctx, err = WithToolContext(ctx, captured)
					if err != nil {
						return nil, err
					}
				}
			}
			return next(ctx, method, request)
		}
	})
	return scopemcp.Register(server, bindings...)
}

func InvocationID(ctx context.Context) (string, error) {
	invocation, err := pluginmcp.InvocationFromContext(ctx)
	return invocation.ID, err
}

// ToolCommandID separates model Tool receipt identities from human command keys
// while preserving the exact logical identity throughout transport retries.
func ToolCommandID(ctx context.Context) (string, error) {
	id, err := InvocationID(ctx)
	if err != nil {
		return "", err
	}
	return toolCommandID(id)
}

func toolCommandID(id string) (string, error) {
	if err := pluginmcp.CommandID(id).Validate(); err != nil {
		return "", err
	}
	command := ToolCommandPrefix + id
	if err := pluginmcp.CommandID(command).Validate(); err != nil {
		return "", err
	}
	return command, nil
}
