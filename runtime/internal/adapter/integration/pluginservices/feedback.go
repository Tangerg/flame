package pluginservices

import (
	"context"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"

	"github.com/Tangerg/qimu/jsonvalue"
)

func (c *CommandCalls) PrepareFeedback(ctx context.Context, input extension.FeedbackDraft) (extension.AnnotationSource, jsonvalue.Value, error) {
	contract, err := pluginsdk.FeedbackContract()
	if err != nil {
		return extension.AnnotationSource{}, jsonvalue.Value{}, err
	}
	method, err := pluginsdk.PrepareFeedbackMethod()
	if err != nil {
		return extension.AnnotationSource{}, jsonvalue.Value{}, err
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
		return extension.AnnotationSource{}, jsonvalue.Value{}, err
	}
	source := extension.AnnotationSource{ContributionSource: extension.ContributionSource{InstallationID: reply.binding.InstallationID.String(), Release: reply.binding.Digest.String(), Contribution: reply.binding.ContributionID.String()}, ContractReference: contract.Descriptor().Reference}
	content, err := jsonvalue.Encode(reply.value)
	return source, content, err
}
