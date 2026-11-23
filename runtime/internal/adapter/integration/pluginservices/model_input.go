package pluginservices

import (
	"context"
	"errors"
	"log/slog"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	"github.com/Tangerg/scope/core/chat"
)

func (o *CallComposition) ResolveInput(ctx context.Context) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	return plugins.ResolveComposition(ctx, o.calls.installations, o.calls.compositions, contract)
}

func (o *CallComposition) ValidateInput(ctx context.Context, bindings []plugins.ContributionBinding) error {
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return err
	}
	return o.validateCompositionBindings(ctx, contract, bindings)
}

func (o *CallComposition) PrepareInput(ctx context.Context, bindings []plugins.ContributionBinding, selection flamemodel.Selection, request *chat.Request) (*chat.Request, error) {
	if err := selection.ValidateExact(); err != nil {
		return nil, err
	}
	if request == nil {
		return nil, errors.New("model input: request is required")
	}
	if err := request.Validate(); err != nil {
		return nil, err
	}
	if _, err := pluginmcp.InvocationFromContext(ctx); err != nil {
		return nil, err
	}
	if err := o.ValidateInput(ctx, bindings); err != nil {
		return nil, err
	}
	contract, err := pluginsdk.ModelInputContract()
	if err != nil {
		return nil, err
	}
	method, err := pluginsdk.ModelInputMethod()
	if err != nil {
		return nil, err
	}
	current := request.Clone()
	for _, binding := range bindings {
		input := pluginsdk.ModelInputRequest{Selection: pluginsdk.ModelSelection{Provider: selection.Provider(), Model: selection.Model(), ReasoningEffort: selection.ReasoningEffort()}, Request: *current.Clone()}
		projected, err := callContribution(ctx, o.calls.pool, binding, contract, method, input)
		if err != nil {
			return nil, err
		}
		result := chat.Request(projected)
		if err := pluginsdk.ValidateModelInput(current, &result); err != nil {
			return nil, err
		}
		if slog.Default().Enabled(ctx, slog.LevelDebug) {
			before, beforeErr := modeladapter.DescribeRequest(current)
			after, afterErr := modeladapter.DescribeRequest(&result)
			if err := errors.Join(beforeErr, afterErr); err != nil {
				slog.DebugContext(ctx, "model input diagnostics unavailable", telemetry.ContributionID(string(binding.ContributionID)), telemetry.Error(err))
			} else {
				invocation, _ := pluginmcp.InvocationFromContext(ctx)
				slog.DebugContext(ctx, "model input projection change", telemetry.CallerID(invocation.ID),
					telemetry.InstallationID(binding.InstallationID), telemetry.ContributionID(string(binding.ContributionID)),
					telemetry.ScopeRequestProjection(), slog.Any("before.bytes", before.Bytes), slog.Any("after.bytes", after.Bytes),
					slog.Any("messages.first_changed", before.FirstChangedMessage(after)),
					slog.Any("system.changed", before.SystemDigest != after.SystemDigest),
					slog.Any("options.changed", before.OptionsDigest != after.OptionsDigest),
					slog.Any("before.messages.sha256", before.MessagesDigest), slog.Any("after.messages.sha256", after.MessagesDigest))
			}
		}
		current = result.Clone()
	}
	return current, nil
}
