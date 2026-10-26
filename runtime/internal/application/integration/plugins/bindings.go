package plugins

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ContributionBinding captures one concrete implementation of a consumer contract.
// Installation availability and composition choice remain with their own owners.
type ContributionBinding struct {
	InstallationID identity.InstallationID
	ContributionID plugin.ContributionID
	Digest         identity.Digest
	ServerID       mcpserver.ID
	ServerName     mcpserver.ServerName
	Export         string
	Contract       jsonvalue.Value
}

func SelectedContribution(ctx context.Context, store Store, selections SelectionStore, required *extension.Capability) (ContributionBinding, bool, error) {
	if required == nil || required.Descriptor().Composition != extension.Selection {
		return ContributionBinding{}, false, plugin.ErrInvalid
	}
	choice, found, err := selections.Get(ctx, required.Descriptor().Reference)
	if err != nil || !found {
		return ContributionBinding{}, false, err
	}
	record := choice.Snapshot()
	snapshot, found, err := store.Get(ctx, record.Installation)
	if err != nil || !found {
		return ContributionBinding{}, false, err
	}
	contribution, declared := snapshot.Selected.Contribution(record.Contribution)
	if !declared || !snapshot.Installation.ServerEnabled(contribution.Server) {
		return ContributionBinding{}, false, nil
	}
	if contribution.Contract != required.Identity() {
		return ContributionBinding{}, false, fmt.Errorf("%w: selected contribution contract differs from the consumer requirement", plugin.ErrUnavailable)
	}
	id, err := snapshot.Installation.ServerID(contribution.Server)
	if err != nil {
		return ContributionBinding{}, false, err
	}
	return ContributionBinding{InstallationID: record.Installation, ContributionID: contribution.ID, Digest: snapshot.Selected.Digest(), ServerID: id, ServerName: contribution.Server, Export: contribution.Export, Contract: contribution.Contract}, true, nil
}
