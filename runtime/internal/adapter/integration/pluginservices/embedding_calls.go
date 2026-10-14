package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

// EmbeddingRoleCallback publishes the configured role on ordinary installed
// callbacks. A feature's optional fallback and work budget stay with its caller.
func EmbeddingRoleCallback(resolve func(context.Context) (models.TextEmbedder, error)) (pluginsdk.Callback, error) {
	if resolve == nil {
		return pluginsdk.Callback{}, errors.New("embedding call: role resolver is required")
	}
	return pluginsdk.NewTextEmbeddingCallback(func(ctx context.Context, command pluginsdk.Command[[]string]) ([][]float32, error) {
		if command.ID == executionctx.InvocationOwner(ctx) {
			return nil, errors.New("embedding call: independent invocation cannot claim its executing owner identity")
		}
		model, err := resolve(ctx)
		if err != nil {
			return nil, err
		}
		if model == nil {
			return nil, &capability.Refusal{Code: "model_role_unconfigured", Message: "embedding call: role is not configured"}
		}
		if dependency.Missing(model) {
			return nil, errors.New("embedding call: resolved role is typed nil")
		}
		return model.Embed(ctx, command.Input)
	})
}
