package agentmemory

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

// CurationStore is the persistence port for automatic memory maintenance. The
// append-only ledger is internal implementation state; only a successful
// Reconcile changes the public agent-memory generation. Calls borrow inputs
// synchronously and transfer ownership of returned facts and items.
type CurationStore interface {
	AppendLedger(ctx context.Context, batch domain.FactBatch) ([]domain.LedgerFact, error)
	PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]domain.LedgerFact, error)
	State(ctx context.Context, project string) (domain.State, error)
	Reconcile(ctx context.Context, publication domain.Publication) (bool, error)
	Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
}

// CurationConfig bundles the automatic-maintenance ports.
type CurationConfig struct {
	Store         CurationStore
	Invalidations invalidation.Publish
}

// Curation owns the append-ledger and generation-publication use cases. It is
// deliberately separate from Coordinator so review consumers cannot invoke
// background maintenance operations.
type Curation struct {
	store         CurationStore
	invalidations invalidation.Publish
}

// NewCuration builds the automatic memory-maintenance use case.
func NewCuration(cfg CurationConfig) (*Curation, error) {
	if dependency.Missing(cfg.Store) {
		return nil, errors.New("agentmemory: curation store is required")
	}
	return &Curation{store: cfg.Store, invalidations: cfg.Invalidations}, nil
}

// AppendLedger records newly extracted facts. The ledger is not a public read
// model, so appending it does not invalidate agentMemory.list.
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

// PendingLedger returns facts not yet incorporated into the curated generation.
// It reads from a curation State rather than a bare sequence so the position it
// resumes at is the one State already proves coherent.
func (c *Curation) PendingLedger(ctx context.Context, project string, state domain.State, limit int) ([]domain.LedgerFact, error) {
	if err := validatePendingRead(project, state, limit); err != nil {
		return nil, err
	}
	return c.store.PendingLedger(ctx, project, state.Watermark, limit)
}

// State returns the current curation watermark.
func (c *Curation) State(ctx context.Context, project string) (domain.State, error) {
	if err := domain.ValidateTarget(domain.ScopeProject, project); err != nil {
		return domain.State{}, err
	}
	return c.store.State(ctx, project)
}

// PublishGeneration publishes one compare-and-swap-protected curated
// generation and invalidates the public projection only for the winning fold.
func (c *Curation) PublishGeneration(ctx context.Context, publication domain.Publication) (bool, error) {
	if err := publication.Validate(); err != nil {
		return false, err
	}
	published, err := c.store.Reconcile(ctx, publication)
	if err != nil {
		return false, err
	}
	if published {
		c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	}
	return published, nil
}

// Items returns the valid active generation used as input to the next fold,
// with pinned and newer values first and identity as the stable tie-breaker.
func (c *Curation) Items(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error) {
	if err := domain.ValidateTarget(scope, project); err != nil {
		return nil, err
	}
	items, err := c.store.Items(ctx, scope, project)
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
