package agentmemory

import (
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

// CurationStore is the persistence port for automatic memory maintenance. The
// append-only ledger is internal implementation state; only a successful
// Reconcile changes the public agent-memory generation.
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
func NewCuration(cfg CurationConfig) *Curation {
	return &Curation{store: cfg.Store, invalidations: cfg.Invalidations}
}

// Available reports whether automatic memory maintenance is wired.
func (c *Curation) Available() bool { return c != nil && c.store != nil }

// AppendLedger records newly extracted facts. The ledger is not a public read
// model, so appending it does not invalidate agentMemory.list.
func (c *Curation) AppendLedger(ctx context.Context, batch domain.FactBatch) ([]domain.LedgerFact, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	normalized, err := batch.Normalize()
	if err != nil {
		return nil, err
	}
	if len(normalized.Facts) == 0 {
		return nil, nil
	}
	storeBatch := normalized
	storeBatch.Facts = slices.Clone(normalized.Facts)
	facts, err := c.store.AppendLedger(ctx, storeBatch)
	if err != nil {
		return nil, err
	}
	if err := validateAppendedFacts(facts, normalized); err != nil {
		return nil, err
	}
	return slices.Clone(facts), nil
}

// PendingLedger returns facts not yet incorporated into the curated generation.
func (c *Curation) PendingLedger(ctx context.Context, project string, watermark int64, limit int) ([]domain.LedgerFact, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	if err := validatePendingRead(project, watermark, limit); err != nil {
		return nil, err
	}
	facts, err := c.store.PendingLedger(ctx, project, watermark, limit)
	if err != nil {
		return nil, err
	}
	if err := validatePendingFacts(facts, watermark, limit); err != nil {
		return nil, err
	}
	return slices.Clone(facts), nil
}

// State returns the current curation watermark.
func (c *Curation) State(ctx context.Context, project string) (domain.State, error) {
	if !c.Available() {
		return domain.State{}, ErrUnavailable
	}
	if err := domain.ValidateTarget(domain.ScopeProject, project); err != nil {
		return domain.State{}, err
	}
	state, err := c.store.State(ctx, project)
	if err != nil {
		return domain.State{}, err
	}
	if err := state.Validate(); err != nil {
		return domain.State{}, fmt.Errorf("agentmemory: invalid curation state for project %q: %w", project, err)
	}
	return state, nil
}

// PublishGeneration publishes one compare-and-swap-protected curated
// generation and invalidates the public projection only for the winning fold.
func (c *Curation) PublishGeneration(ctx context.Context, publication domain.Publication) (bool, error) {
	if !c.Available() {
		return false, ErrUnavailable
	}
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
	if !c.Available() {
		return nil, ErrUnavailable
	}
	if err := domain.ValidateTarget(scope, project); err != nil {
		return nil, err
	}
	items, err := c.store.Items(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	items = cloneItems(items)
	if err := validateActiveTargetCatalog(items, scope, project); err != nil {
		return nil, err
	}
	slices.SortFunc(items, compareActiveItems)
	return items, nil
}

func validateAppendedFacts(facts []domain.LedgerFact, batch domain.FactBatch) error {
	if len(facts) > len(batch.Facts) {
		return fmt.Errorf("agentmemory: append returned %d facts for a %d-fact batch", len(facts), len(batch.Facts))
	}
	remaining := make(map[string]struct{}, len(batch.Facts))
	for _, content := range batch.Facts {
		remaining[content] = struct{}{}
	}
	var previous int64
	for index, fact := range facts {
		if err := fact.Validate(); err != nil {
			return fmt.Errorf("agentmemory: appended ledger row %d is invalid: %w", index+1, err)
		}
		if fact.Sequence <= previous {
			return fmt.Errorf("agentmemory: appended ledger sequence %d is not after %d", fact.Sequence, previous)
		}
		previous = fact.Sequence
		if fact.Day != batch.Day || !fact.CapturedAt.Equal(batch.CapturedAt) {
			return fmt.Errorf("agentmemory: appended ledger row %d does not acknowledge its batch", index+1)
		}
		if _, requested := remaining[fact.Content]; !requested {
			return fmt.Errorf("agentmemory: appended ledger row %d was not requested", index+1)
		}
		delete(remaining, fact.Content)
	}
	return nil
}

func validatePendingRead(project string, watermark int64, limit int) error {
	if err := domain.ValidateTarget(domain.ScopeProject, project); err != nil {
		return err
	}
	if watermark < 0 {
		return fmt.Errorf("agentmemory: curation watermark must not be negative")
	}
	if limit <= 0 || limit > domain.MaxLedgerFoldFacts {
		return fmt.Errorf("agentmemory: pending ledger limit must be between 1 and %d", domain.MaxLedgerFoldFacts)
	}
	return nil
}

func validatePendingFacts(facts []domain.LedgerFact, watermark int64, limit int) error {
	if len(facts) > limit {
		return fmt.Errorf("agentmemory: pending ledger returned %d facts, limit %d", len(facts), limit)
	}
	previous := watermark
	for index, fact := range facts {
		if err := fact.Validate(); err != nil {
			return fmt.Errorf("agentmemory: pending ledger row %d is invalid: %w", index+1, err)
		}
		if fact.Sequence <= previous {
			return fmt.Errorf("agentmemory: pending ledger sequence %d is not after %d", fact.Sequence, previous)
		}
		previous = fact.Sequence
	}
	return nil
}
