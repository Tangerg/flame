package plugins

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/jsoninput"
)

// ContributionBinding captures one concrete implementation of a consumer contract.
// Installation availability and composition choice remain with their own owners.
type ContributionBinding struct {
	InstallationID resourceid.InstallationID
	ContributionID plugin.ContributionID
	Digest         fingerprint.Digest
	ServerID       mcpserver.ID
	ServerName     mcpserver.ServerName
	Export         string
	Contract       jsoninput.Input
}

func SelectedContribution(ctx context.Context, store Store, selections SelectionStore, required *capability.Contract) (ContributionBinding, bool, error) {
	if required == nil || required.Descriptor().Composition != capability.Selection {
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
