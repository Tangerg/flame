package pluginservices

import (
	"context"
	"errors"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"

	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

type ToolPreparationConfig struct {
	Installations plugins.Store
	Callbacks     *pluginmcp.Callbacks
	Embeddings    func(context.Context) (models.TextEmbedder, error)
	Invalidations invalidation.Publish
}

func NewToolPreparation(config ToolPreparationConfig) (mcp.ToolPreparation, error) {
	memoryContract, err := plugincontract.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return nil, err
	}
	if lo.IsNil(config.Installations) || config.Callbacks == nil || config.Embeddings == nil {
		return nil, errors.New("plugin tools: installations, callbacks and embedding resolver are required")
	}
	return func(ctx context.Context, executable mcp.Executable, host mcp.ToolHost) (mcp.ToolCall, func() error, error) {
		source := executable.SourceConfig().Server
		snapshot, installed, err := admitInstalledSource(ctx, config.Installations, source)
		if err != nil {
			return nil, nil, err
		}
		if !installed {
			return func(invocation toolcontract.Invocation) (toolcontract.Result, error) {
				return executable.Call(ctx, invocation)
			}, func() error { return nil }, nil
		}
		if snapshot.Selected.HasImplementation(source.Name, memoryContract) {
			return prepareMemoryTool(ctx, executable, config.Callbacks, config.Embeddings, config.Invalidations)
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

func prepareInstalledTool(ctx context.Context, executable mcp.Executable, host mcp.ToolHost, invalidations invalidation.Publish) (mcp.ToolCall, func() error, error) {
	metadata, release, err := host(ctx)
	if err != nil {
		return nil, nil, err
	}
	return installedToolCall(ctx, executable.WithCallMetadata(metadata), invalidations), release, nil
}

func installedToolCall(ctx context.Context, executable mcp.Executable, invalidations invalidation.Publish) mcp.ToolCall {
	return func(invocation toolcontract.Invocation) (toolcontract.Result, error) {
		defer invalidations.Notify(invalidation.Notice{Resource: invalidation.PluginData})
		return executable.Call(ctx, invocation)
	}
}

// Only the publisher's search Tool receives Memory's narrow embedding grant.
func prepareMemoryTool(ctx context.Context, source mcp.Executable, callbacks *pluginmcp.Callbacks, embeddings func(context.Context) (models.TextEmbedder, error), invalidations invalidation.Publish) (mcp.ToolCall, func() error, error) {
	cwd, attached := executionctx.WorkspaceCWD(ctx)
	if !attached {
		return nil, nil, errors.New("memory: Run workspace is required")
	}
	ref, found, err := mcp.IdentifyTool(source)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		return nil, nil, errors.New("memory: Tool identity is required")
	}
	_, remote, _ := ref.MCP()
	if remote.String() != memorycontract.SearchToolName {
		metadata := sdk.ParamsBase{}
		if err := metadata.Set(pluginsdk.ToolContextMetadataKey, pluginsdk.ToolContext{CWD: cwd}); err != nil {
			return nil, nil, err
		}
		return installedToolCall(ctx, source.WithCallMetadata(metadata), invalidations), func() error { return nil }, nil
	}
	embedder, err := embeddings(ctx)
	if err != nil {
		return nil, nil, err
	}
	closeEmbedder := func() error {
		if embedder != nil {
			return embedder.Close(context.WithoutCancel(ctx))
		}
		return nil
	}
	borrowed, err := source.BorrowSession(ctx)
	if err != nil {
		return nil, nil, errors.Join(err, closeEmbedder())
	}
	ctx = borrowed.Context()
	grant, revoke, err := GrantMemoryModels(ctx, callbacks, borrowed.Session(), nil, embedder)
	if err != nil {
		return nil, nil, errors.Join(err, borrowed.Close(), closeEmbedder())
	}
	release := func() error { revoke(); return errors.Join(borrowed.Close(), closeEmbedder()) }
	space := ""
	if embedder != nil {
		space = embedder.ID()
	}
	metadata, err := pluginmcp.CallbackMetadata(grant)
	if err != nil {
		return nil, nil, errors.Join(err, release())
	}
	if err := metadata.Set(pluginsdk.ToolContextMetadataKey, pluginsdk.ToolContext{CWD: cwd, EmbeddingSpace: space}); err != nil {
		return nil, nil, errors.Join(err, release())
	}
	return installedToolCall(ctx, source.WithCallMetadata(metadata), invalidations), release, nil
}
