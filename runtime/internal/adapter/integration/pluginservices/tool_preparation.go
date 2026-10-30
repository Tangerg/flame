package pluginservices

import (
	"context"
	"errors"
	"maps"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	toolcontract "github.com/Tangerg/scope/core/tool"
	scopemcp "github.com/Tangerg/scope/mcp"
	"github.com/samber/lo"
)

type ToolPreparationConfig struct {
	Installations plugins.Store
	Callbacks     *qimumcp.Callbacks
	Embeddings    func(context.Context) (models.TextEmbedder, error)
	Invalidations invalidation.Publish
}

func NewToolPreparation(config ToolPreparationConfig) (mcp.ToolPreparation, error) {
	memoryContract, err := portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
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
	scoped := scopeCallbacks(ctx, metadata)
	return installedToolCall(scoped, executable, invalidations), release, nil
}

func installedToolCall(ctx context.Context, executable mcp.Executable, invalidations invalidation.Publish) mcp.ToolCall {
	return func(invocation toolcontract.Invocation) (toolcontract.Result, error) {
		defer invalidations.Notify(invalidation.Notice{Resource: invalidation.PluginData})
		return executable.Call(ctx, invocation)
	}
}

func scopeCallbacks(ctx context.Context, metadata sdk.ParamsBase) context.Context {
	combined := scopemcp.RequestMetaFromContext(ctx)
	if combined == nil {
		combined = make(sdk.Meta)
	}
	maps.Copy(combined, metadata.Meta)
	return scopemcp.WithRequestMeta(ctx, combined)
}

// Only the publisher's search Tool receives Memory's narrow embedding grant.
func prepareMemoryTool(ctx context.Context, source mcp.Executable, callbacks *qimumcp.Callbacks, embeddings func(context.Context) (models.TextEmbedder, error), invalidations invalidation.Publish) (mcp.ToolCall, func() error, error) {
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
		// Scope ignores empty metadata overrides, so retain the mandatory Tool
		// context before withdrawing an inherited callback grant.
		scoped, err := pluginsdk.WithToolContext(ctx, pluginsdk.ToolContext{CWD: cwd})
		if err != nil {
			return nil, nil, err
		}
		metadata := scopemcp.RequestMetaFromContext(scoped)
		delete(metadata, qimumcp.CallbackMetadataKey)
		scoped = scopemcp.WithRequestMeta(scoped, metadata)
		return installedToolCall(scoped, source, invalidations), func() error { return nil }, nil
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
	metadata, err := qimumcp.CallbackMetadata(grant)
	if err != nil {
		return nil, nil, errors.Join(err, release())
	}
	scoped, err := pluginsdk.WithToolContext(scopeCallbacks(ctx, metadata), pluginsdk.ToolContext{CWD: cwd, EmbeddingSpace: space})
	if err != nil {
		return nil, nil, errors.Join(err, release())
	}
	return installedToolCall(scoped, source, invalidations), release, nil
}
