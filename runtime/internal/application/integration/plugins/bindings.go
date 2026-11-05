package plugins

import (
	"context"
	"fmt"
	"slices"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/frame"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

// ContributionBinding captures one concrete implementation of a consumer contract.
// Installation availability and composition choice remain with their own owners.
type ContributionBinding struct {
	InstallationID identity.InstallationID
	ContributionID plugin.ContributionID
	Digest         framevalue.Digest
	ServerID       mcpserver.ID
	ServerName     mcpserver.ServerName
	Export         string
	Contract       framevalue.JSON
	Activation     *plugin.Activation
}

// SameContribution compares the consumer's declared source independently of
// the original execution graph attached after capture or checkpoint restoration.
func (b ContributionBinding) SameContribution(other ContributionBinding) bool {
	b.Activation = nil
	other.Activation = nil
	return b == other
}

func SelectedContribution(ctx context.Context, host *frame.Host, store Store, selections SelectionStore, required *plugincontract.Capability) (ContributionBinding, bool, error) {
	if required == nil {
		return ContributionBinding{}, false, plugin.ErrInvalid
	}
	binding, state, err := SelectedContributionForRole(ctx, host, store, selections, required, required.Descriptor().Reference)
	return binding, state == SelectionActive, err
}

type SelectionState uint8

const (
	SelectionAbsent SelectionState = iota
	SelectionInactive
	SelectionActive
)

func SelectedContributionForRole(ctx context.Context, host *frame.Host, store Store, selections SelectionStore, required *plugincontract.Capability, role string) (ContributionBinding, SelectionState, error) {
	if host == nil || required == nil || required.Descriptor().Composition != plugincontract.Selection {
		return ContributionBinding{}, SelectionAbsent, plugin.ErrInvalid
	}
	choice, found, err := selections.Get(ctx, role)
	if err != nil || !found {
		return ContributionBinding{}, SelectionAbsent, err
	}
	binding, available, err := resolveContribution(ctx, host, store, required, choice.Snapshot(), framevalue.Digest{})
	if !available {
		return binding, SelectionInactive, err
	}
	return binding, SelectionActive, err
}

func CapturedContribution(ctx context.Context, host *frame.Host, store Store, required *plugincontract.Capability, source extension.ContributionSource) (ContributionBinding, bool, error) {
	if err := source.Validate(); err != nil {
		return ContributionBinding{}, false, err
	}
	installation, err := identity.ParseInstallation(source.InstallationID)
	if err != nil {
		return ContributionBinding{}, false, err
	}
	release, err := framevalue.ParseDigest(source.Release)
	if err != nil {
		return ContributionBinding{}, false, err
	}
	record := plugin.SelectionRecord{Role: required.Descriptor().Reference, Installation: installation, Contribution: plugin.ContributionID(source.Contribution)}
	return resolveContribution(ctx, host, store, required, record, release)
}

func (b ContributionBinding) ContributionSource() extension.ContributionSource {
	return extension.ContributionSource{InstallationID: b.InstallationID.String(), Release: b.Digest.String(), Contribution: b.ContributionID.String()}
}

func resolveContribution(ctx context.Context, host *frame.Host, store Store, required *plugincontract.Capability, record plugin.SelectionRecord, release framevalue.Digest) (ContributionBinding, bool, error) {
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
	installed, found := host.Installation(source.InstallationID)
	if !found || !installed.Enabled || installed.Release != source.Release {
		return ContributionBinding{}, false, plugin.ErrUnavailable
	}
	return binding, true, nil
}
