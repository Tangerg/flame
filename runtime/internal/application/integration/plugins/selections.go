package plugins

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/samber/lo"
)

type SelectionStore interface {
	Get(context.Context, string) (plugin.ProviderSelection, bool, error)
	List(context.Context) ([]plugin.ProviderSelection, error)
	Save(context.Context, plugin.ProviderSelection) error
}

type connectorSelection interface {
	WithdrawExternal() error
	ReconcileExternal(context.Context) error
}
type Selections struct {
	connector     connectorSelection
	store         SelectionStore
	installations Store
	transact      func(context.Context, func(context.Context) error) error
	publish       invalidation.Publish
}

func NewSelections(store SelectionStore, installations Store, transact func(context.Context, func(context.Context) error) error, publish invalidation.Publish, connector connectorSelection) (*Selections, error) {
	if lo.IsNil(store) || lo.IsNil(installations) || transact == nil || lo.IsNil(connector) {
		return nil, errors.New("plugins: provider selections, installations and admission are required")
	}
	return &Selections{connector: connector, store: store, installations: installations, transact: transact, publish: publish}, nil
}

func (s *Selections) List(ctx context.Context) ([]plugin.ProviderSelection, error) {
	return s.store.List(ctx)
}

func (s *Selections) Select(ctx context.Context, record plugin.SelectionRecord) (plugin.ProviderSelection, error) {
	selection, err := plugin.NewProviderSelection(record)
	if err != nil {
		return plugin.ProviderSelection{}, err
	}
	err = s.transact(ctx, func(ctx context.Context) error {
		snapshot, found, err := s.installations.Get(ctx, record.Installation)
		if err != nil {
			return err
		}
		if !found {
			return plugin.ErrNotFound
		}
		contribution, found := snapshot.Selected.Contribution(record.Contribution)
		if !found {
			return fmt.Errorf("%w: contribution is not declared", plugin.ErrInvalid)
		}
		if !slices.Contains(contribution.SelectionRoles(), record.Role) {
			return fmt.Errorf("%w: contribution does not provide the requested selection role", plugin.ErrInvalid)
		}
		if err := s.store.Save(ctx, selection); err != nil {
			return err
		}
		if record.Role == mcpconnector.Reference {
			return s.connector.WithdrawExternal()
		}
		return nil
	})
	if err == nil {
		s.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
		if record.Role == mcpconnector.Reference {
			err = s.connector.ReconcileExternal(context.WithoutCancel(ctx))
		}
	}
	return selection, err
}
