package plugins

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/qimu"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/qimu/integrity"
	"github.com/Tangerg/qimu/jsonvalue"
	"github.com/Tangerg/qimu/portable"
)

// ContributionBinding captures one concrete implementation of a consumer contract.
// Installation availability and composition choice remain with their own owners.
type ContributionBinding struct {
	InstallationID identity.InstallationID
	ContributionID plugin.ContributionID
	Digest         integrity.Digest
	ServerID       mcpserver.ID
	ServerName     mcpserver.ServerName
	Export         string
	Contract       jsonvalue.Value
	Activation     *plugin.Activation
}

// SameContribution compares the consumer's declared source independently of
// the original execution graph attached after capture or checkpoint restoration.
func (b ContributionBinding) SameContribution(other ContributionBinding) bool {
	b.Activation = nil
	other.Activation = nil
	return b == other
}

func SelectedContribution(ctx context.Context, host *qimu.Host, store Store, selections SelectionStore, required *portable.Capability) (ContributionBinding, bool, error) {
	if required == nil {
		return ContributionBinding{}, false, plugin.ErrInvalid
	}
	return SelectedContributionForRole(ctx, host, store, selections, required, required.Descriptor().Reference)
}

func SelectedContributionForRole(ctx context.Context, host *qimu.Host, store Store, selections SelectionStore, required *portable.Capability, role string) (ContributionBinding, bool, error) {
	if host == nil || required == nil || required.Descriptor().Composition != portable.Selection {
		return ContributionBinding{}, false, plugin.ErrInvalid
	}
	choice, found, err := selections.Get(ctx, role)
	if err != nil || !found {
		return ContributionBinding{}, false, err
	}
	record := choice.Snapshot()
	return resolveContribution(ctx, host, store, required, record, integrity.Digest{})
}

func CapturedContribution(ctx context.Context, host *qimu.Host, store Store, required *portable.Capability, source extension.ContributionSource) (ContributionBinding, bool, error) {
	if err := source.Validate(); err != nil {
		return ContributionBinding{}, false, err
	}
	installation, err := identity.ParseInstallation(source.InstallationID)
	if err != nil {
		return ContributionBinding{}, false, err
	}
	release, err := integrity.ParseDigest(source.Release)
	if err != nil {
		return ContributionBinding{}, false, err
	}
	record := plugin.SelectionRecord{Role: required.Descriptor().Reference, Installation: installation, Contribution: plugin.ContributionID(source.Contribution)}
	return resolveContribution(ctx, host, store, required, record, release)
}

func (b ContributionBinding) ContributionSource() extension.ContributionSource {
	return extension.ContributionSource{InstallationID: b.InstallationID.String(), Release: b.Digest.String(), Contribution: b.ContributionID.String()}
}

func resolveContribution(ctx context.Context, host *qimu.Host, store Store, required *portable.Capability, record plugin.SelectionRecord, release integrity.Digest) (ContributionBinding, bool, error) {
	snapshot, found, err := store.Get(ctx, record.Installation)
	if err != nil || !found {
		return ContributionBinding{}, false, err
	}
	if !release.IsZero() && release != snapshot.Selected.Digest() {
		return ContributionBinding{}, false, plugin.ErrUnavailable
	}
	contribution, declared := snapshot.Selected.Contribution(record.Contribution)
	if !declared || !snapshot.Installation.ServerEnabled(contribution.Server) {
		return ContributionBinding{}, false, nil
	}
	if contribution.Contract != required.Identity() || !slices.Contains(contribution.SelectionRoles(), record.Role) {
		return ContributionBinding{}, false, fmt.Errorf("%w: selected contribution contract differs from the consumer requirement", plugin.ErrUnavailable)
	}
	id, err := snapshot.Installation.ServerID(contribution.Server)
	if err != nil {
		return ContributionBinding{}, false, err
	}
	binding := ContributionBinding{InstallationID: record.Installation, ContributionID: contribution.ID, Digest: snapshot.Selected.Digest(), ServerID: id, ServerName: contribution.Server, Export: contribution.Export, Contract: contribution.Contract}
	source, err := ContributionSource(binding)
	if err != nil {
		return ContributionBinding{}, false, err
	}
	if err := host.CheckSource(ctx, source); err != nil {
		return ContributionBinding{}, false, FrameworkError(err)
	}
	return binding, true, nil
}
