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

// ErrUnavailable reports that a requested agent-memory capability is not wired.
var ErrUnavailable = errors.New("agentmemory: unavailable")

// RootResolver is the narrow workspace dependency this use case consumes.
// Its implementation belongs to the workspace application component; the
// agent-memory package does not learn filesystem or path-normalization details.
type RootResolver interface {
	ResolveRoot(cwd string) (string, error)
}

// Store is the review-oriented persistence port consumed by this coordinator.
// List returns one complete target without presentation ordering; the
// coordinator owns that cross-item policy. Extraction and search declare their
// own narrower consumer views.
type Store interface {
	List(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	Review(ctx context.Context, id domain.ItemID, decision domain.ReviewDecision, now time.Time) error
	Update(ctx context.Context, id domain.ItemID, content *string, pinned *bool, now time.Time) (domain.Item, error)
	Delete(ctx context.Context, id domain.ItemID) error
	Add(ctx context.Context, scope domain.Scope, project, content string, now time.Time) (item domain.Item, changed bool, err error)
}

// Config bundles the review use case's driven ports. Store may be nil to
// disable review. Roots is required only for project-scoped requests; a
// missing resolver reports an explicit unavailable error.
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

// New builds the coordinator. Nil stores are valid disabled states so
// capability negotiation and optional maintenance remain truthful.
func New(cfg Config) *Coordinator {
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{store: cfg.Store, roots: cfg.Roots, now: now, invalidations: cfg.Invalidations}
}

// Available reports whether agent-memory review operations are wired.
func (c *Coordinator) Available() bool { return c != nil && c.store != nil }

// List returns active and pending memory items for scope/cwd.
func (c *Coordinator) List(ctx context.Context, scope domain.Scope, cwd string) ([]domain.Item, error) {
	if !c.Available() {
		return nil, ErrUnavailable
	}
	project, err := c.project(scope, cwd)
	if err != nil {
		return nil, err
	}
	items, err := c.store.List(ctx, scope, project)
	if err != nil {
		return nil, err
	}
	items = cloneItems(items)
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
	if !c.Available() {
		return ErrUnavailable
	}
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
	if !c.Available() {
		return domain.Item{}, ErrUnavailable
	}
	if content != nil {
		value := *content
		content = &value
	}
	if pinned != nil {
		value := *pinned
		pinned = &value
	}
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
	item = item.Clone()
	if err := validateUpdatedItem(item, itemID, content, pinned); err != nil {
		return domain.Item{}, err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return item, nil
}

// Delete removes one memory item.
func (c *Coordinator) Delete(ctx context.Context, id string) error {
	if !c.Available() {
		return ErrUnavailable
	}
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
	if !c.Available() {
		return domain.Item{}, ErrUnavailable
	}
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
	item = item.Clone()
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
	if c.roots == nil {
		return "", ErrUnavailable
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
