package plugins

import (
	"context"
	"errors"
	"fmt"
	"github.com/Tangerg/flame/qimu"

	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/samber/lo"
)

type SelectionStore interface {
	Get(context.Context, string) (plugin.ProviderSelection, bool, error)
	List(context.Context) ([]plugin.ProviderSelection, error)
	Save(context.Context, plugin.ProviderSelection) error
}

type Selections struct {
	store         SelectionStore
	installations Store
	admission     *qimu.Admission
	publish       invalidation.Publish
}

func NewSelections(store SelectionStore, installations Store, admission *qimu.Admission, publish invalidation.Publish) (*Selections, error) {
	if lo.IsNil(store) || lo.IsNil(installations) || admission == nil {
		return nil, errors.New("plugins: provider selections, installations and admission are required")
	}
	return &Selections{store: store, installations: installations, admission: admission, publish: publish}, nil
}

func (s *Selections) List(ctx context.Context) ([]plugin.ProviderSelection, error) {
	return s.store.List(ctx)
}

func (s *Selections) Select(ctx context.Context, record plugin.SelectionRecord) (plugin.ProviderSelection, error) {
	selection, err := plugin.NewProviderSelection(record)
	if err != nil {
		return plugin.ProviderSelection{}, err
	}
	err = s.admission.Under(ctx, func() error {
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
		contract, err := portable.ParseCapability(contribution.Contract.JSON())
		if err != nil {
			return err
		}
		descriptor := contract.Descriptor()
		if descriptor.Reference != record.Role || descriptor.Composition != portable.Selection {
			return fmt.Errorf("%w: contribution does not provide the requested selection role", plugin.ErrInvalid)
		}
		return s.store.Save(ctx, selection)
	})
	if err == nil {
		s.publish.Notify(invalidation.Notice{Resource: invalidation.Plugins})
	}
	return selection, err
}
