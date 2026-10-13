package rpc

import (
	"context"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	scopemcp "github.com/Tangerg/scope/mcp"
)

const ToolScopeKey = "io.github.tangerg.flame/memory-scope.v1"

type ToolScope struct {
	CWD, EmbeddingSpace string
	Grant               pluginsdk.CallbackRef
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
