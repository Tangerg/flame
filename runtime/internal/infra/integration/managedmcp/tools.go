package managedmcp

import (
	"context"
	json "encoding/json/v2"
	"errors"

	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const ToolScopeKey = "io.github.tangerg.flame/memory-scope.v1"
const InvocationKey = "io.github.tangerg.flame/invocationId"

type ToolScope struct{ CWD, Grant, EmbeddingSpace string }

func RegisterTools(server *sdk.Server, tools ...tool.Tool) error {
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
func MemoryScope(ctx context.Context) (ToolScope, error) {
	encoded, err := json.Marshal(scopemcp.RequestMetaFromContext(ctx)[ToolScopeKey])
	if err != nil {
		return ToolScope{}, err
	}
	var scope ToolScope
	if err := json.Unmarshal(encoded, &scope); err != nil {
		return scope, err
	}
	if scope.CWD == "" {
		return scope, errors.New("memory: captured Run workspace is required")
	}
	return scope, nil
}
func InvocationID(ctx context.Context) (string, error) {
	id, _ := scopemcp.RequestMetaFromContext(ctx)[InvocationKey].(string)
	if id == "" || len(id) > 256 {
		return "", errors.New("managed plugin: bounded logical invocation identity is required")
	}
	return "tool:" + id, nil
}
