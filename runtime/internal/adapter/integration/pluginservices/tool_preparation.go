package pluginservices

import (
	"context"
	"errors"
	"maps"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices/memorycontract"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
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
		id, installed := source.Source.Origin().Installation()
		if !installed {
			return func(invocation toolcontract.Invocation) (chat.ToolOutput, error) {
				return executable.Call(ctx, invocation)
			}, func() {}, nil
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
		if snapshot.Selected.HasImplementation(source.Name, memoryContract) {
			return prepareMemoryTool(ctx, executable, config.Callbacks, config.Embeddings)
		}
		return prepareInstalledTool(ctx, executable, host, config.Invalidations)
	}, nil
}

// prepareInstalledTool gives an ordinary installed backend the same canonical
// call-scoped host grant its public plugins.call entry receives, so one business
// reached through either entry keeps identical host capability and call
// attribution. The grant's release is owned by the caller; a write invalidates
// projected reads on return.
func prepareInstalledTool(ctx context.Context, executable mcp.Executable, host mcpconnection.ToolHost, invalidations invalidation.Publish) (mcpconnection.ToolCall, func(), error) {
	if host == nil {
		return nil, nil, errors.New("plugin tools: installed host grant is required")
	}
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

// scopeCallbacks carries an opened grant's callback reference into the tool request
// metadata the backend admits, leaving the calling-owner invocation untouched.
func scopeCallbacks(ctx context.Context, metadata sdk.ParamsBase) context.Context {
	combined := scopemcp.RequestMetaFromContext(ctx)
	if combined == nil {
		combined = make(sdk.Meta)
	}
	maps.Copy(combined, metadata.Meta)
	return scopemcp.WithRequestMeta(ctx, combined)
}

// prepareMemoryTool keeps Memory's own narrow model/embedding grant and its
// CWD/embedding-space scope; Memory owns that two-call policy and does not receive
// the canonical installed grant.
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
