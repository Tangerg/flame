package pluginservices

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	framevalue "github.com/Tangerg/frame/value"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
)

func (c *CommandCalls) PrepareFeedback(ctx context.Context, input extension.FeedbackDraft) (extension.AnnotationSource, framevalue.JSON, error) {
	contract, err := pluginsdk.FeedbackContract()
	if err != nil {
		return extension.AnnotationSource{}, framevalue.JSON{}, err
	}
	method, err := pluginsdk.PrepareFeedbackMethod()
	if err != nil {
		return extension.AnnotationSource{}, framevalue.JSON{}, err
	}
	reply, err := invokeResolvedCommand[extension.FeedbackDraft, pluginsdk.FeedbackSignal](ctx, c, method.Name(), input, nil, func(ctx context.Context) (plugins.ContributionBinding, error) {
		binding, found, err := plugins.SelectedContribution(ctx, c.framework, c.installations, c.selections, contract)
		if err != nil {
			return binding, err
		}
		if !found {
			return binding, plugin.ErrUnavailable
		}
		if expected := input.Source; expected != nil && (binding.InstallationID.String() != expected.InstallationID || binding.Digest.String() != expected.Release || binding.ContributionID.String() != expected.Contribution) {
			return binding, plugin.ErrStale
		}
		return binding, nil
	}, nil)
	if err != nil {
		return extension.AnnotationSource{}, framevalue.JSON{}, err
	}
	source := extension.AnnotationSource{ContributionSource: extension.ContributionSource{InstallationID: reply.binding.InstallationID.String(), Release: reply.binding.Digest.String(), Contribution: reply.binding.ContributionID.String()}, ContractReference: contract.Descriptor().Reference}
	content, err := framevalue.EncodeJSON(reply.value)
	return source, content, err
}
