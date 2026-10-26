package plugins

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/samber/lo"
)

type CompositionStore interface {
	Get(context.Context, string) (plugin.OrderedComposition, bool, error)
	List(context.Context) ([]plugin.OrderedComposition, error)
	Save(context.Context, plugin.OrderedComposition) error
}

type Compositions struct {
	store         CompositionStore
	installations Store
	admission     *Admission
	publish       invalidation.Publish
}

func NewCompositions(store CompositionStore, installations Store, admission *Admission, publish invalidation.Publish) (*Compositions, error) {
	if lo.IsNil(store) || lo.IsNil(installations) || admission == nil {
		return nil, errors.New("plugins: compositions, installations and admission are required")
	}
	return &Compositions{store: store, installations: installations, admission: admission, publish: publish}, nil
}

func (c *Compositions) List(ctx context.Context) ([]plugin.OrderedComposition, error) {
	return c.store.List(ctx)
}

func (c *Compositions) Resolve(ctx context.Context, required *extension.Capability) ([]ContributionBinding, error) {
	var bindings []ContributionBinding
	err := c.admission.Under(ctx, func() error {
		var err error
		bindings, err = ResolveComposition(ctx, c.installations, c.store, required)
		return err
	})
	return bindings, err
}

func (c *Compositions) Set(ctx context.Context, record plugin.CompositionRecord) (plugin.OrderedComposition, error) {
	value, err := plugin.NewOrderedComposition(record)
	if err != nil {
		return plugin.OrderedComposition{}, err
	}
	err = c.admission.Under(ctx, func() error {
		if _, err := bindComposition(ctx, c.installations, value); err != nil {
			return err
		}
		return c.store.Save(ctx, value)
	})
	if err == nil {
		c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	}
	return value, err
}

// ResolveComposition freezes the exact selected releases in stored order. A
// missing or withdrawn member refuses the chain rather than silently shortening it.
func ResolveComposition(ctx context.Context, installations Store, compositions CompositionStore, required *extension.Capability) ([]ContributionBinding, error) {
	if required == nil || required.Descriptor().Composition != extension.Decoration {
		return nil, plugin.ErrInvalid
	}
	value, found, err := compositions.Get(ctx, required.Descriptor().Reference)
	if err != nil {
		return nil, err
	}
	if !found {
		return []ContributionBinding{}, nil
	}
	if value.Snapshot().Contract != required.Identity() {
		return nil, fmt.Errorf("%w: stored composition contract differs from the consumer requirement", plugin.ErrUnavailable)
	}
	return bindComposition(ctx, installations, value)
}

func bindComposition(ctx context.Context, installations Store, composition plugin.OrderedComposition) ([]ContributionBinding, error) {
	return resolveCompositionMembers(composition, func(id identity.InstallationID) (plugin.Snapshot, bool, error) {
		return installations.Get(ctx, id)
	})
}

func resolveCompositionMembers(composition plugin.OrderedComposition, snapshotFor func(identity.InstallationID) (plugin.Snapshot, bool, error)) ([]ContributionBinding, error) {
	if err := composition.Validate(); err != nil {
		return nil, err
	}
	record := composition.Snapshot()
	bindings := make([]ContributionBinding, 0, len(record.Members))
	for _, member := range record.Members {
		snapshot, found, err := snapshotFor(member.Installation)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("%w: composition member installation is absent", plugin.ErrUnavailable)
		}
		contribution, found := snapshot.Selected.Contribution(member.Contribution)
		if !found || contribution.Contract != record.Contract || !snapshot.Installation.ServerEnabled(contribution.Server) {
			return nil, fmt.Errorf("%w: composition member is not admitted", plugin.ErrUnavailable)
		}
		id, err := snapshot.Installation.ServerID(contribution.Server)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, ContributionBinding{InstallationID: member.Installation, ContributionID: member.Contribution, Digest: snapshot.Selected.Digest(), ServerID: id, ServerName: contribution.Server, Export: contribution.Export, Contract: contribution.Contract})
	}
	return bindings, nil
}
