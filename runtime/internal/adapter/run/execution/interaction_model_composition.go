package execution

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/jsoninput"
	modelchoice "github.com/Tangerg/flame/runtime/model"
	"github.com/Tangerg/flame/runtime/resourceid"
	"github.com/Tangerg/scope/core/chat"
)

type InteractionCallComposition interface {
	Pending([]plugins.ContributionBinding) bool
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
	return append(bindings, i.toolProposals...)
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

func decodeContributionBindings(values []contributionBindingWire, contract *capability.Contract) ([]plugins.ContributionBinding, error) {
	if contract == nil {
		return nil, errors.New("execution: model composition contract is required")
	}
	bindings := make([]plugins.ContributionBinding, 0, len(values))
	seen := map[plugin.CompositionMember]bool{}
	for _, value := range values {
		installation, err := resourceid.ParseInstallation(value.Installation)
		if err != nil {
			return nil, err
		}
		digest, err := fingerprint.ParseDigest(value.Digest)
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
		identity, err := jsoninput.Parse([]byte(value.Contract))
		if err != nil || identity != contract.Identity() {
			return nil, errors.Join(err, errors.New("execution: model contribution contract changed"))
		}
		member := plugin.CompositionMember{Installation: installation, Contribution: contribution}
		if seen[member] || !capability.ValidExportName(value.Export) {
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
