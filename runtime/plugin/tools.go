package plugin

import (
	"context"
	"errors"
	"unicode/utf8"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const InvocationKey = "io.github.tangerg.flame/invocationId"

// RegisterTools retains Scope's frozen Tool contracts and failure semantics.
// MCP metadata carries Runtime's logical identity; model arguments never own it.
func RegisterTools(server *sdk.Server, tools ...tool.Tool) error {
	if server == nil {
		return errors.New("plugin: server is required")
	}
	server.AddReceivingMiddleware(func(next sdk.MethodHandler) sdk.MethodHandler {
		return func(ctx context.Context, method string, request sdk.Request) (sdk.Result, error) {
			if call, ok := request.(*sdk.CallToolRequest); ok && call.Params != nil {
				ctx = scopemcp.WithRequestMeta(ctx, call.Params.Meta)
			}
			return next(ctx, method, request)
		}
	})
	return scopemcp.Register(server, tools...)
}

func InvocationID(ctx context.Context) (string, error) {
	id, _ := scopemcp.RequestMetaFromContext(ctx)[InvocationKey].(string)
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return "", errors.New("plugin: bounded logical invocation identity is required")
	}
	return id, nil
}

// ToolCommandID separates model Tool receipt identities from human command keys
// while preserving the exact logical identity throughout transport retries.
func ToolCommandID(ctx context.Context) (string, error) {
	id, err := InvocationID(ctx)
	if err != nil {
		return "", err
	}
	command := "tool:" + id
	if err := validateCommandID(command); err != nil {
		return "", err
	}
	return command, nil
}
