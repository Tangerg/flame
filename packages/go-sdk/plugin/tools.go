package plugin

import (
	"context"
	"errors"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"

	"github.com/Tangerg/flame/qimu/jsonvalue"
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
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
			if call, ok := request.(*sdk.CallToolRequest); ok && call.Params != nil {
				arguments := call.Params.Arguments
				if len(arguments) == 0 {
					arguments = []byte("{}")
				}
				input, err := jsonvalue.Parse(arguments)
				if err != nil {
					return nil, err
				}
				ctx, err = qimumcp.AdmitRequest(ctx, input, call.Params.Meta)
				if err != nil {
					return nil, err
				}
				ctx = scopemcp.WithRequestMeta(ctx, call.Params.Meta)
			}
			return next(ctx, method, request)
		}
	})
	return scopemcp.Register(server, tools...)
}

func InvocationID(ctx context.Context) (string, error) {
	invocation, err := qimumcp.InvocationFromContext(ctx)
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
	if err := qimumcp.ValidateCommandID(id); err != nil {
		return "", err
	}
	command := ToolCommandPrefix + id
	if err := qimumcp.ValidateCommandID(command); err != nil {
		return "", err
	}
	return command, nil
}
