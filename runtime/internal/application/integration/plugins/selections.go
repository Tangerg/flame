package plugins

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/samber/lo"
)

type SelectionStore interface {
	Get(context.Context, string) (plugin.ProviderSelection, bool, error)
	List(context.Context) ([]plugin.ProviderSelection, error)
	Save(context.Context, plugin.ProviderSelection) error
}

type connectorSelection interface {
	ApplyExternalSelection(context.Context, func(context.Context) (bool, error)) error
}
type Selections struct {
	connector     connectorSelection
	store         SelectionStore
	installations Store
	transact      func(context.Context, func(context.Context) error) error
	publish       invalidation.Publish
}

func NewSelections(store SelectionStore, installations Store, transact func(context.Context, func(context.Context) error) error, publish invalidation.Publish, connector connectorSelection) (*Selections, error) {
	if lo.IsNil(store) || lo.IsNil(installations) || transact == nil || lo.IsNil(connector) || publish == nil {
		return nil, errors.New("plugins: provider selections, installations and admission are required")
	}
	return &Selections{connector: connector, store: store, installations: installations, transact: transact, publish: publish}, nil
}

func (s *Selections) List(ctx context.Context) ([]plugin.ProviderSelection, error) {
	return s.store.List(ctx)
}

func (s *Selections) Select(ctx context.Context, record plugin.SelectionRecord, complete operation.Reply[plugin.ProviderSelection]) error {
	selection, err := plugin.NewProviderSelection(record)
	if err != nil {
		return err
	}
	var changed bool
	commit := func(ctx context.Context) (bool, error) {
		err := complete(ctx, func(tx context.Context) (plugin.ProviderSelection, error) {
			err := s.transact(tx, func(ctx context.Context) error {
				selected, err := s.saveSelection(ctx, selection)
				changed = selected
				return err
			})
			return selection, err
		})
		return changed, err
	}
	if record.Role == mcpconnector.Reference {
		err = s.connector.ApplyExternalSelection(ctx, commit)
	} else {
		_, err = commit(ctx)
	}
	if err == nil && changed {
		s.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	}
	return err
}

func (s *Selections) saveSelection(ctx context.Context, selection plugin.ProviderSelection) (bool, error) {
	record := selection.Snapshot()
	snapshot, found, err := s.installations.Get(ctx, record.Installation)
	if err != nil {
		return false, err
	}
	if !found {
		return false, plugin.ErrNotFound
	}
	contribution, found := snapshot.Selected.Contribution(record.Contribution)
	if !found {
		return false, fmt.Errorf("%w: contribution is not declared", plugin.ErrInvalid)
	}
	if !slices.Contains(contribution.SelectionRoles(), record.Role) {
		return false, fmt.Errorf("%w: contribution does not provide the requested selection role", plugin.ErrInvalid)
	}
	current, found, err := s.store.Get(ctx, record.Role)
	if err != nil || found && current == selection {
		return false, err
	}
	if err := s.store.Save(ctx, selection); err != nil {
		return false, err
	}
	return true, nil
}
