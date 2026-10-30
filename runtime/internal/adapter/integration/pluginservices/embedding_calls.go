package pluginservices

import (
	"context"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/samber/lo"
)

// EmbeddingRoleCallback publishes the configured role on ordinary installed
// callbacks. A feature's optional fallback and work budget stay with its caller.
func EmbeddingRoleCallback(resolve func(context.Context) (models.TextEmbedder, error)) (qimumcp.Callback, error) {
	if resolve == nil {
		return qimumcp.Callback{}, errors.New("embedding call: role resolver is required")
	}
	return pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command qimumcp.Command[[]string]) (result pluginsdk.TextEmbeddings, returned error) {
		if command.ID == executionctx.InvocationOwner(ctx) {
			return pluginsdk.TextEmbeddings{}, errors.New("embedding call: independent invocation cannot claim its executing owner identity")
		}
		model, err := resolve(ctx)
		if err != nil {
			return pluginsdk.TextEmbeddings{}, err
		}
		if model == nil {
			return pluginsdk.TextEmbeddings{}, &portable.Refusal{Code: "model_role_unconfigured", Message: "embedding call: role is not configured"}
		}
		if lo.IsNil(model) {
			return pluginsdk.TextEmbeddings{}, errors.New("embedding call: resolved role is typed nil")
		}
		defer func() { returned = errors.Join(returned, model.Close(context.WithoutCancel(ctx))) }()
		vectors, err := model.Embed(ctx, command.Input)
		return pluginsdk.TextEmbeddings{Space: model.ID(), Vectors: vectors}, err
	})
}
