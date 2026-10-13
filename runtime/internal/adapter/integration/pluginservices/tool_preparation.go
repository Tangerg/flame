package pluginservices

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	app "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	schedulerpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

type ToolPreparationConfig struct {
	Installations plugins.Store
	Callbacks     *pluginsdk.Callbacks
	Embeddings    func(context.Context) (app.Embedder, error)
	Invalidations invalidation.Publish
}

func NewToolPreparation(config ToolPreparationConfig) (mcpconnection.ToolPreparation, error) {
	memoryContracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	schedulesContracts, err := schedulerpc.NewContracts()
	if err != nil {
		return nil, err
	}

	if dependency.Missing(config.Installations) || config.Callbacks == nil || config.Embeddings == nil {
		return nil, errors.New("plugin tools: installations, callbacks and embedding resolver are required")
	}
	return func(ctx context.Context, executable mcp.Executable) (mcpconnection.ToolCall, func(), error) {
		call := func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
			return executable.Call(ctx, invocation)
		}
		source := executable.SourceConfig().Server
		id, installed := source.Source.Origin().Installation()
		if !installed {
			return call, func() {}, nil
		}
		snapshot, found, err := config.Installations.Get(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if !found || !snapshot.Installation.ServerEnabled(source.Name) {
			return nil, nil, plugin.ErrUnavailable
		}
		current, err := snapshot.Installation.ServerSource(snapshot.Selected, source.Name)
		if err != nil {
			return nil, nil, err
		}
		if current != source.Source {
			return nil, nil, plugin.ErrUnavailable
		}
		if snapshot.Selected.HasImplementation(source.Name, memoryContracts.Capability) {
			scoped, release, err := prepareMemoryTool(ctx, executable, config.Callbacks, config.Embeddings)
			if err != nil {
				return nil, nil, err
			}
			return func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
				return executable.Call(scoped, invocation)
			}, release, nil
		}
		if snapshot.Selected.HasImplementation(source.Name, schedulesContracts.Capability) {
			// A lost reply may follow a commit. This notice asks for a new owner
			// read and supplies no competing success or business state.
			return func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
				defer config.Invalidations.Notify(invalidation.ForSchedules())
				return call(invocation)
			}, func() {}, nil
		}
		return call, func() {}, nil
	}, nil
}

func prepareMemoryTool(ctx context.Context, source mcp.Executable, callbacks *pluginsdk.Callbacks, embeddings func(context.Context) (app.Embedder, error)) (context.Context, func(), error) {
	cwd, attached := executionctx.WorkspaceCWD(ctx)
	if !attached {
		return nil, nil, errors.New("memory: Run workspace is required")
	}
	embedder, err := embeddings(ctx)
	if err != nil {
		return nil, nil, err
	}
	session, err := source.CallbackSession()
	if err != nil {
		return nil, nil, err
	}
	grant, release, err := rpc.GrantModels(ctx, callbacks, session, nil, embedder)
	if err != nil {
		return nil, nil, err
	}
	space := ""
	if embedder != nil {
		space = embedder.ID()
	}
	metadata := maps.Clone(scopemcp.RequestMetaFromContext(ctx))
	if metadata == nil {
		metadata = make(map[string]any)
	}
	metadata[rpc.ToolScopeKey] = rpc.ToolScope{CWD: cwd, Grant: grant, EmbeddingSpace: space}
	return scopemcp.WithRequestMeta(ctx, metadata), release, nil
}
