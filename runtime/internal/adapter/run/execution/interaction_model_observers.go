package execution

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

type InteractionModelObservers interface {
	Resolve(context.Context) ([]plugins.ContributionBinding, error)
	Validate(context.Context, []plugins.ContributionBinding) error
	HeldDependencies() []plugin.Dependency
	Bind(chat.Model, chat.Streamer, []plugins.ContributionBinding) (chat.Model, chat.Streamer)
}

type modelObserverWire struct {
	Installation string `json:"installation"`
	Digest       string `json:"digest"`
	Contribution string `json:"contribution"`
	Server       string `json:"server"`
	Export       string `json:"export"`
	Contract     string `json:"contract"`
}

func observerBindingsWire(bindings []plugins.ContributionBinding) []modelObserverWire {
	values := make([]modelObserverWire, len(bindings))
	for index, binding := range bindings {
		values[index] = modelObserverWire{Installation: binding.InstallationID.String(), Digest: binding.Digest.String(), Contribution: binding.ContributionID.String(), Server: binding.ServerName.String(), Export: binding.Export, Contract: binding.Contract.String()}
	}
	return values
}

func decodeObserverBindings(values []modelObserverWire) ([]plugins.ContributionBinding, error) {
	contract, err := pluginsdk.ModelObserverContract()
	if err != nil {
		return nil, err
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
			return nil, errors.Join(err, errors.New("execution: observer contract changed"))
		}
		member := plugin.CompositionMember{Installation: installation, Contribution: contribution}
		if seen[member] || !capability.ValidExportName(value.Export) {
			return nil, errors.New("execution: invalid observer binding")
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
