package coordination

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
	"github.com/samber/lo"
)

type CurationStore interface {
	AppendLedger(ctx context.Context, batch agentmemory.FactBatch) ([]agentmemory.LedgerFact, error)
	PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]agentmemory.LedgerFact, error)
	State(ctx context.Context, project string) (agentmemory.State, error)
	Reconcile(ctx context.Context, publication agentmemory.Publication) (bool, error)
	GenerationItems(ctx context.Context, project string) ([]agentmemory.Item, error)
}

type Curation struct {
	store CurationStore
}

func NewCuration(store CurationStore) (*Curation, error) {
	if lo.IsNil(store) {
		return nil, errors.New("agentmemory: curation store is required")
	}
	return &Curation{store: store}, nil
}

func (c *Curation) AppendLedger(ctx context.Context, batch agentmemory.FactBatch) ([]agentmemory.LedgerFact, error) {
	normalized, err := batch.Normalize()
	if err != nil {
		return nil, err
	}
	if len(normalized.Facts) == 0 {
		return nil, nil
	}
	return c.store.AppendLedger(ctx, normalized)
}

func (c *Curation) PendingLedger(ctx context.Context, project string, state agentmemory.State, limit int) ([]agentmemory.LedgerFact, error) {
	if err := validatePendingRead(project, state, limit); err != nil {
		return nil, err
	}
	return c.store.PendingLedger(ctx, project, state.Watermark, limit)
}

func (c *Curation) State(ctx context.Context, project string) (agentmemory.State, error) {
	if err := agentmemory.ValidateTarget(agentmemory.ScopeProject, project); err != nil {
		return agentmemory.State{}, err
	}
	return c.store.State(ctx, project)
}

func (c *Curation) PublishGeneration(ctx context.Context, publication agentmemory.Publication) (bool, error) {
	if err := publication.Validate(); err != nil {
		return false, err
	}
	published, err := c.store.Reconcile(ctx, publication)
	if err != nil {
		return false, err
	}
	return published, nil
}

func (c *Curation) GenerationItems(ctx context.Context, project string) ([]agentmemory.Item, error) {
	if err := agentmemory.ValidateTarget(agentmemory.ScopeProject, project); err != nil {
		return nil, err
	}
	items, err := c.store.GenerationItems(ctx, project)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(items, compareActiveItems)
	return items, nil
}

func validatePendingRead(project string, state agentmemory.State, limit int) error {
	if err := agentmemory.ValidateTarget(agentmemory.ScopeProject, project); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if limit <= 0 || limit > agentmemory.MaxLedgerFoldFacts {
		return fmt.Errorf("agentmemory: pending ledger limit must be between 1 and %d", agentmemory.MaxLedgerFoldFacts)
	}
	return nil
}
