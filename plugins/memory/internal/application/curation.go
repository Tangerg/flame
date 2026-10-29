package coordination

import (
	"context"
	"errors"
	"fmt"
	"slices"

	domain "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/samber/lo"
)

// Ledger appends do not publish a generation; only a winning Reconcile invalidates public reads.
type CurationStore interface {
	AppendLedger(ctx context.Context, batch domain.FactBatch) ([]domain.LedgerFact, error)
	PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]domain.LedgerFact, error)
	State(ctx context.Context, project string) (domain.State, error)
	Reconcile(ctx context.Context, publication domain.Publication) (bool, error)
	GenerationItems(ctx context.Context, project string) ([]domain.Item, error)
}

type CurationConfig struct {
	Store    CurationStore
	OnChange func()
}

type Curation struct {
	store    CurationStore
	onChange func()
}

func NewCuration(cfg CurationConfig) (*Curation, error) {
	if lo.IsNil(cfg.Store) {
		return nil, errors.New("agentmemory: curation store is required")
	}
	return &Curation{store: cfg.Store, onChange: cfg.OnChange}, nil
}

func (c *Curation) AppendLedger(ctx context.Context, batch domain.FactBatch) ([]domain.LedgerFact, error) {
	normalized, err := batch.Normalize()
	if err != nil {
		return nil, err
	}
	if len(normalized.Facts) == 0 {
		return nil, nil
	}
	return c.store.AppendLedger(ctx, normalized)
}

func (c *Curation) PendingLedger(ctx context.Context, project string, state domain.State, limit int) ([]domain.LedgerFact, error) {
	if err := validatePendingRead(project, state, limit); err != nil {
		return nil, err
	}
	return c.store.PendingLedger(ctx, project, state.Watermark, limit)
}

func (c *Curation) State(ctx context.Context, project string) (domain.State, error) {
	if err := domain.ValidateTarget(domain.ScopeProject, project); err != nil {
		return domain.State{}, err
	}
	return c.store.State(ctx, project)
}

func (c *Curation) PublishGeneration(ctx context.Context, publication domain.Publication) (bool, error) {
	if err := publication.Validate(); err != nil {
		return false, err
	}
	published, err := c.store.Reconcile(ctx, publication)
	if err != nil {
		return false, err
	}
	if published {
		if c.onChange != nil {
			c.onChange()
		}
	}
	return published, nil
}

func (c *Curation) GenerationItems(ctx context.Context, project string) ([]domain.Item, error) {
	if err := domain.ValidateTarget(domain.ScopeProject, project); err != nil {
		return nil, err
	}
	items, err := c.store.GenerationItems(ctx, project)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, compareActiveItems)
	return items, nil
}

func validatePendingRead(project string, state domain.State, limit int) error {
	if err := domain.ValidateTarget(domain.ScopeProject, project); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if limit <= 0 || limit > domain.MaxLedgerFoldFacts {
		return fmt.Errorf("agentmemory: pending ledger limit must be between 1 and %d", domain.MaxLedgerFoldFacts)
	}
	return nil
}
