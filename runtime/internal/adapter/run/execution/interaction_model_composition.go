package execution

import (
	"context"
	"errors"
	"slices"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/scope/core/chat"
)

type interactionCallComposition interface {
	Pending([]plugins.ContributionBinding) bool
	RestorationFailure([]plugins.ContributionBinding) (bool, error)
	ResolveToolProposals(context.Context) ([]plugins.ContributionBinding, error)
	ValidateToolProposals(context.Context, []plugins.ContributionBinding) error
	PrepareToolProposal(context.Context, []plugins.ContributionBinding, chat.ToolCall) (chat.ToolCall, error)
	Activations(context.Context, []plugins.ContributionBinding) ([]plugin.Activation, error)
	ResolveInput(context.Context) ([]plugins.ContributionBinding, error)
	ValidateInput(context.Context, []plugins.ContributionBinding) error
	PrepareInput(context.Context, []plugins.ContributionBinding, modelchoice.Selection, *chat.Request) (*chat.Request, error)
	ResolveContext(context.Context) ([]plugins.ContributionBinding, error)
	ValidateContext(context.Context, []plugins.ContributionBinding) error
	PrepareContext(context.Context, []plugins.ContributionBinding, modelchoice.Selection, *chat.Request, []chat.Message) ([]chat.Message, error)
	Resolve(context.Context) ([]plugins.ContributionBinding, error)
	Validate(context.Context, []plugins.ContributionBinding) error
	HeldDependencies() []plugin.Dependency
	Bind(chat.Model, chat.Streamer, []plugins.ContributionBinding) (chat.Model, chat.Streamer)
}

func (i *interactionSession) compositionBindings() []plugins.ContributionBinding {
	bindings := append([]plugins.ContributionBinding{}, i.modelObservers...)
	bindings = append(bindings, i.modelContexts...)
	bindings = append(bindings, i.modelInputs...)
	bindings = append(bindings, i.toolProposals...)
	if capture, found := i.selectedChat.Capture(); found {
		bindings = append(bindings, capture.Binding)
	}
	return bindings
}

type contributionBindingWire struct {
	Installation string `json:"installation"`
	Digest       string `json:"digest"`
	Contribution string `json:"contribution"`
	Server       string `json:"server"`
	Export       string `json:"export"`
	Contract     string `json:"contract"`
}

func contributionBindingsWire(bindings []plugins.ContributionBinding) []contributionBindingWire {
	values := make([]contributionBindingWire, len(bindings))
	for index, binding := range bindings {
		values[index] = contributionBindingWire{Installation: binding.InstallationID.String(), Digest: binding.Digest.String(), Contribution: binding.ContributionID.String(), Server: binding.ServerName.String(), Export: binding.Export, Contract: binding.Contract.String()}
	}
	return values
}

func decodeContributionBindings(values []contributionBindingWire, contract *portable.Capability) ([]plugins.ContributionBinding, error) {
	if contract == nil {
		return nil, errors.New("execution: model composition contract is required")
	}
	bindings := make([]plugins.ContributionBinding, 0, len(values))
	seen := map[plugin.CompositionMember]bool{}
	for _, value := range values {
		installation, err := flameidentity.ParseInstallation(value.Installation)
		if err != nil {
			return nil, err
		}
		digest, err := flameidentity.ParseDigest(value.Digest)
		if err != nil {
			return nil, err
		}
		contribution, err := plugin.ParseContributionID(value.Contribution)
		if err != nil {
			return nil, err
		}
		server, err := mcpserver.ParseServerName(value.Server)
		if err != nil {
			return nil, err
		}
		identity, err := jsonvalue.Parse([]byte(value.Contract))
		if err != nil || identity != contract.Identity() {
			return nil, errors.Join(err, errors.New("execution: model contribution contract changed"))
		}
		member := plugin.CompositionMember{Installation: installation, Contribution: contribution}
		if seen[member] || !portable.ValidExportName(value.Export) {
			return nil, errors.New("execution: invalid model contribution binding")
		}
		seen[member] = true
		origin, err := mcpserver.InstallationOrigin(installation)
		if err != nil {
			return nil, err
		}
		id, err := mcpserver.NewID(origin, server)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, plugins.ContributionBinding{InstallationID: installation, Digest: digest, ContributionID: contribution, ServerName: server, ServerID: id, Export: value.Export, Contract: identity})
	}
	return bindings, nil
}

func activationIdentities(activations []plugin.Activation) []string {
	identities := make([]string, len(activations))
	for i, activation := range activations {
		identities[i] = activation.Identity.String()
	}
	return identities
}

func (i *interactionSession) bindCompositionActivations(activations []plugin.Activation) error {
	var err error
	i.modelObservers, err = plugins.BindActivations(i.modelObservers, activations)
	if err != nil {
		return err
	}
	i.modelContexts, err = plugins.BindActivations(i.modelContexts, activations)
	if err != nil {
		return err
	}
	i.modelInputs, err = plugins.BindActivations(i.modelInputs, activations)
	if err != nil {
		return err
	}
	i.toolProposals, err = plugins.BindActivations(i.toolProposals, activations)
	if err != nil {
		return err
	}
	var selected []plugin.Activation
	for _, binding := range i.compositionBindings() {
		if !slices.ContainsFunc(selected, binding.Activation.Equal) {
			selected = append(selected, binding.Activation.Clone())
		}
	}
	i.state.mu.Lock()
	i.compositionActivations = selected
	i.state.mu.Unlock()
	return nil
}
