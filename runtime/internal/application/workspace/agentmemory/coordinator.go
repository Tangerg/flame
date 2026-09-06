// Package agentmemory owns review, curation, and search use cases for
// agent-maintained memory.
package agentmemory

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

// RootResolver is the narrow workspace dependency this use case consumes.
// Its implementation belongs to the workspace application component; the
// agent-memory package does not learn filesystem or path-normalization details.
type RootResolver interface {
	ResolveRoot(cwd string) (string, error)
}

// Store is the review-oriented persistence port consumed by this coordinator.
// List returns one complete target without presentation ordering; the
// coordinator owns that cross-item policy. Extraction and search declare their
// own narrower consumer views. Calls borrow inputs synchronously; returned items
// and collections transfer ownership to the caller.
type Store interface {
	List(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	Review(ctx context.Context, id domain.ItemID, decision domain.ReviewDecision, now time.Time) error
	Update(ctx context.Context, id domain.ItemID, content *string, pinned *bool, now time.Time) (domain.Item, error)
	Delete(ctx context.Context, id domain.ItemID) error
	Add(ctx context.Context, scope domain.Scope, project, content string, now time.Time) (item domain.Item, changed bool, err error)
}

// Config supplies the persistence and workspace dependencies required for review.
type Config struct {
	Store         Store
	Roots         RootResolver
	Now           func() time.Time
	Invalidations invalidation.Publish
}

// Coordinator implements agent-memory review commands and queries.
type Coordinator struct {
	store         Store
	roots         RootResolver
	now           func() time.Time
	invalidations invalidation.Publish
}

// New constructs the complete review use case before it can accept requests.
func New(cfg Config) (*Coordinator, error) {
	if nilDependency(cfg.Store) {
		return nil, errors.New("agentmemory: review store is required")
	}
	if nilDependency(cfg.Roots) {
		return nil, errors.New("agentmemory: workspace resolver is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{store: cfg.Store, roots: cfg.Roots, now: now, invalidations: cfg.Invalidations}, nil
}

// List returns active and pending memory items for scope/cwd.
func (c *Coordinator) List(ctx context.Context, scope domain.Scope, cwd string) ([]domain.Item, error) {
	project, err := c.project(scope, cwd)
	if err != nil {
		return nil, err
	}
	items, err := c.store.List(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	if err := validateManagementTargetCatalog(items, scope, project); err != nil {
		return nil, err
	}
	slices.SortFunc(items, compareManagementItems)
	return items, nil
}

func compareManagementItems(a, b domain.Item) int {
	if a.Status != b.Status {
		if a.Status == domain.StatusPending {
			return -1
		}
		return 1
	}
	return compareActiveItems(a, b)
}

func compareActiveItems(a, b domain.Item) int {
	if a.Pinned != b.Pinned {
		if a.Pinned {
			return -1
		}
		return 1
	}
	if order := b.UpdatedAt.Compare(a.UpdatedAt); order != 0 {
		return order
	}
	return cmp.Compare(b.ID.String(), a.ID.String())
}

// Review accepts or rejects an extracted proposal.
func (c *Coordinator) Review(ctx context.Context, id string, decision domain.ReviewDecision) error {
	if _, err := decision.Result(); err != nil {
		return err
	}
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return err
	}
	if err := c.store.Review(ctx, itemID, decision, c.now()); err != nil {
		return err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return nil
}

// Update applies the content/pin patch as one use case and returns the saved
// item. The persistence port commits both requested fields atomically.
func (c *Coordinator) Update(ctx context.Context, id string, content *string, pinned *bool) (domain.Item, error) {
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return domain.Item{}, err
	}
	if content != nil {
		canonical, err := domain.NormalizeContent(*content)
		if err != nil {
			return domain.Item{}, err
		}
		content = &canonical
	}
	item, err := c.store.Update(ctx, itemID, content, pinned, c.now())
	if err != nil {
		return domain.Item{}, err
	}
	if err := validateUpdatedItem(item, itemID, content, pinned); err != nil {
		return domain.Item{}, err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return item, nil
}

// Delete removes one memory item.
func (c *Coordinator) Delete(ctx context.Context, id string) error {
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return err
	}
	if err := c.store.Delete(ctx, itemID); err != nil {
		return err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return nil
}

// Add creates an immediately-active user-authored memory item.
func (c *Coordinator) Add(ctx context.Context, scope domain.Scope, cwd, content string) (domain.Item, error) {
	project, err := c.project(scope, cwd)
	if err != nil {
		return domain.Item{}, err
	}
	content, err = domain.NormalizeContent(content)
	if err != nil {
		return domain.Item{}, err
	}
	item, changed, err := c.store.Add(ctx, scope, project, content, c.now())
	if err != nil {
		return domain.Item{}, err
	}
	if err := validateAddedItem(item, scope, project, content); err != nil {
		return domain.Item{}, err
	}
	if changed {
		c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	}
	return item, nil
}

func (c *Coordinator) project(scope domain.Scope, cwd string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", err
	}
	if scope == domain.ScopeUser {
		return "", nil
	}
	project, err := c.roots.ResolveRoot(cwd)
	if err != nil {
		return "", err
	}
	if err := domain.ValidateTarget(scope, project); err != nil {
		return "", err
	}
	return project, nil
}
