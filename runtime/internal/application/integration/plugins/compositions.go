package plugins

import (
	"context"
	"errors"
	"fmt"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/samber/lo"
)

type CompositionStore interface {
	Get(context.Context, string) (plugin.OrderedComposition, bool, error)
	List(context.Context) ([]plugin.OrderedComposition, error)
	Save(context.Context, plugin.OrderedComposition) error
}

type CompositionsConfig struct {
	Store         CompositionStore
	Installations Store
	ReadSnapshot  func(context.Context, func(context.Context) error) error
	Transact      func(context.Context, func(context.Context) error) error
	Publish       invalidation.Publish
}

type Compositions struct {
	store         CompositionStore
	installations Store
	readSnapshot  func(context.Context, func(context.Context) error) error
	transact      func(context.Context, func(context.Context) error) error
	publish       invalidation.Publish
}

func NewCompositions(cfg CompositionsConfig) (*Compositions, error) {
	if lo.IsNil(cfg.Store) || lo.IsNil(cfg.Installations) || cfg.ReadSnapshot == nil || cfg.Transact == nil || cfg.Publish == nil {
		return nil, errors.New("plugins: compositions, installations, transactions and invalidation are required")
	}
	return &Compositions{
		store: cfg.Store, installations: cfg.Installations, readSnapshot: cfg.ReadSnapshot,
		transact: cfg.Transact, publish: cfg.Publish,
	}, nil
}

func (c *Compositions) List(ctx context.Context) ([]plugin.OrderedComposition, error) {
	return c.store.List(ctx)
}

func (c *Compositions) Resolve(ctx context.Context, required *plugincontract.Capability) ([]ContributionBinding, error) {
	var bindings []ContributionBinding
	err := c.readSnapshot(ctx, func(ctx context.Context) error {
		var err error
		bindings, err = ResolveComposition(ctx, c.installations, c.store, required)
		return err
	})
	return bindings, err
}

func (c *Compositions) Set(ctx context.Context, value plugin.OrderedComposition, reply operation.Reply[plugin.OrderedComposition]) error {
	if !value.Valid() {
		return plugin.ErrInvalid
	}
	err := c.transact(ctx, func(ctx context.Context) error {
		if _, err := bindComposition(ctx, c.installations, value); err != nil {
			return err
		}
		return reply(ctx, func(tx context.Context) (plugin.OrderedComposition, error) {
			return value, c.store.Save(tx, value)
		})
	})
	if err == nil {
		c.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	}
	return err
}

func ResolveComposition(ctx context.Context, installations Store, compositions CompositionStore, required *plugincontract.Capability) ([]ContributionBinding, error) {
	if required == nil || required.Descriptor().Composition != plugincontract.Decoration {
		return nil, plugin.ErrInvalid
	}
	value, found, err := compositions.Get(ctx, required.Descriptor().Reference)
	if err != nil {
		return nil, err
	}
	if !found {
		return []ContributionBinding{}, nil
	}
	if value.ContractIdentity() != required.Identity() {
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
	contract, members := composition.ContractIdentity(), composition.Members()
	bindings := make([]ContributionBinding, 0, len(members))
	for _, member := range members {
		snapshot, found, err := snapshotFor(member.Installation)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, fmt.Errorf("%w: composition member installation is absent", plugin.ErrUnavailable)
		}
		contribution, found := snapshot.Selected.Contribution(member.Contribution)
		if !found || contribution.Contract != contract || !snapshot.Installation.ServerEnabled(contribution.Server) {
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
