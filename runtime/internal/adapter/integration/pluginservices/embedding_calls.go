package pluginservices

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	framecall "github.com/Tangerg/frame/call"
)

func EmbeddingRoleCallback(resolve func(context.Context) (*modeladapter.Embedder, bool, error)) (pluginmcp.Callback, error) {
	if resolve == nil {
		return pluginmcp.Callback{}, errors.New("embedding call: role resolver is required")
	}
	return pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command pluginmcp.Command[[]string]) (result pluginsdk.TextEmbeddings, returned error) {
		if string(command.ID) == executionctx.InvocationOwner(ctx) {
			return pluginsdk.TextEmbeddings{}, errors.New("embedding call: independent invocation cannot claim its executing owner identity")
		}
		model, configured, err := resolve(ctx)
		if err != nil {
			return pluginsdk.TextEmbeddings{}, err
		}
		if !configured {
			return pluginsdk.TextEmbeddings{}, &framecall.Refusal{Code: refusalModelRoleUnconfigured, Message: "embedding call: role is not configured"}
		}
		defer func() { returned = errors.Join(returned, model.Close(context.WithoutCancel(ctx))) }()
		vectors, err := model.Embed(ctx, command.Input)
		return pluginsdk.TextEmbeddings{Space: model.ID(), Vectors: vectors}, err
	})
}
