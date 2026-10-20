package pluginservices

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices/memorycontract"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
)

type ToolPreparationConfig struct {
	Installations plugins.Store
	Callbacks     *pluginsdk.Callbacks
	Embeddings    func(context.Context) (models.TextEmbedder, error)
	Invalidations invalidation.Publish
}

func NewToolPreparation(config ToolPreparationConfig) (mcpconnection.ToolPreparation, error) {
	memoryContract, err := capability.Parse([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return nil, err
	}
	if dependency.Missing(config.Installations) || config.Callbacks == nil || config.Embeddings == nil {
		return nil, errors.New("plugin tools: installations, callbacks and embedding resolver are required")
	}
	return func(ctx context.Context, executable mcp.Executable, host mcpconnection.ToolHost) (mcpconnection.ToolCall, func(), error) {
		source := executable.SourceConfig().Server
		snapshot, installed, err := admitInstalledSource(ctx, config.Installations, source)
		if err != nil {
			return nil, nil, err
		}
		if !installed {
			return func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
				return executable.Call(ctx, invocation)
			}, func() {}, nil
		}
		if snapshot.Selected.HasImplementation(source.Name, memoryContract) {
			return prepareMemoryTool(ctx, executable, config.Callbacks, config.Embeddings)
		}
		return prepareInstalledTool(ctx, executable, host, config.Invalidations)
	}, nil
}

// installed is false only for a non-plugin MCP server, which carries no Flame host
// grant; a plugin source whose selected release changed since publication is refused.
func admitInstalledSource(ctx context.Context, store plugins.Store, source mcpserver.Server) (plugin.Snapshot, bool, error) {
	id, ok := source.Source.Origin().Installation()
	if !ok {
		return plugin.Snapshot{}, false, nil
	}
	snapshot, found, err := store.Get(ctx, id)
	if err != nil {
		return plugin.Snapshot{}, false, err
	}
	if !found || !snapshot.Installation.ServerEnabled(source.Name) {
		return plugin.Snapshot{}, false, plugin.ErrUnavailable
	}
	current, err := snapshot.Installation.ServerSource(snapshot.Selected, source.Name)
	if err != nil {
		return plugin.Snapshot{}, false, err
	}
	if current != source.Source {
		return plugin.Snapshot{}, false, plugin.ErrUnavailable
	}
	return snapshot, true, nil
}

func prepareInstalledTool(ctx context.Context, executable mcp.Executable, host mcpconnection.ToolHost, invalidations invalidation.Publish) (mcpconnection.ToolCall, func(), error) {
	metadata, release, err := host(ctx)
	if err != nil {
		return nil, nil, err
	}
	scoped := scopeCallbacks(ctx, metadata)
	return func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		defer invalidations.Notify(invalidation.Notice{Resource: invalidation.PluginData})
		return executable.Call(scoped, invocation)
	}, release, nil
}

func scopeCallbacks(ctx context.Context, metadata sdk.ParamsBase) context.Context {
	combined := scopemcp.RequestMetaFromContext(ctx)
	if combined == nil {
		combined = make(sdk.Meta)
	}
	maps.Copy(combined, metadata.Meta)
	return scopemcp.WithRequestMeta(ctx, combined)
}

// Memory owns a narrow two-call model/embedding grant, deliberately not the
// canonical installed grant the ordinary Tool path receives.
func prepareMemoryTool(ctx context.Context, source mcp.Executable, callbacks *pluginsdk.Callbacks, embeddings func(context.Context) (models.TextEmbedder, error)) (mcpconnection.ToolCall, func(), error) {
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
	grant, release, err := GrantMemoryModels(ctx, callbacks, session, nil, embedder)
	if err != nil {
		return nil, nil, err
	}
	space := ""
	if embedder != nil {
		space = embedder.ID()
	}
	metadata, err := pluginsdk.CallbackMetadata(grant)
	if err != nil {
		release()
		return nil, nil, err
	}
	scoped, err := pluginsdk.WithToolContext(scopeCallbacks(ctx, metadata), pluginsdk.ToolContext{CWD: cwd, EmbeddingSpace: space})
	if err != nil {
		release()
		return nil, nil, err
	}
	return func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
		return source.Call(scoped, invocation)
	}, release, nil
}
