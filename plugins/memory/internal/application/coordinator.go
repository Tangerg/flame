package coordination

import (
	"context"
	"errors"
	"time"

	domain "github.com/Tangerg/flame/plugins/memory/internal/domain"
	"github.com/Tangerg/flame/plugins/memory/internal/management"
	"github.com/samber/lo"
)

var ErrWorkspaceUnavailable = errors.New("memory: workspace is unavailable")

var ErrInvalidInput = errors.New("memory: invalid input")

type RootResolver interface {
	ResolveExistingDir(cwd string) (string, error)
}

// Returned items transfer ownership so read ordering cannot mutate store state.
type Store interface {
	Get(ctx context.Context, id domain.ItemID) (domain.Item, bool, error)
	List(ctx context.Context, scope domain.Scope, project string) ([]domain.Item, error)
	PrepareReview(ctx context.Context, id domain.ItemID, expectedRevision uint64, decision domain.ReviewDecision, now time.Time) (management.Mutation, error)
	PrepareUpdate(ctx context.Context, id domain.ItemID, expectedRevision uint64, content *string, pinned *bool, now time.Time) (management.Mutation, error)
	PrepareDelete(ctx context.Context, id domain.ItemID, expectedRevision uint64) (management.Mutation, error)
	PrepareAdd(ctx context.Context, scope domain.Scope, project, content string, now time.Time) (management.Mutation, error)
}

type Config struct {
	Store Store
	Roots RootResolver
	Now   func() time.Time
}

type Coordinator struct {
	store Store
	roots RootResolver
	now   func() time.Time
}

func New(cfg Config) (*Coordinator, error) {
	if lo.IsNil(cfg.Store) {
		return nil, errors.New("agentmemory: review store is required")
	}
	if lo.IsNil(cfg.Roots) {
		return nil, errors.New("agentmemory: workspace resolver is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Coordinator{store: cfg.Store, roots: cfg.Roots, now: now}, nil
}

func (c *Coordinator) Item(ctx context.Context, scope domain.Scope, cwd, id string) (domain.Item, error) {
	project, err := c.project(scope, cwd)
	if err != nil {
		return domain.Item{}, err
	}
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return domain.Item{}, errors.Join(ErrInvalidInput, err)
	}
	item, found, err := c.store.Get(ctx, itemID)
	if err != nil {
		return domain.Item{}, err
	}
	if !found || item.Scope() != scope || item.Project() != project || item.ValidateVisible() != nil {
		return domain.Item{}, domain.ErrNotFound
	}
	return item, nil
}

func (c *Coordinator) PrepareReview(ctx context.Context, id string, expectedRevision uint64, decision domain.ReviewDecision) (management.Mutation, error) {
	if _, err := decision.Result(); err != nil {
		return management.Mutation{}, errors.Join(ErrInvalidInput, err)
	}
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return management.Mutation{}, errors.Join(ErrInvalidInput, err)
	}
	return c.store.PrepareReview(ctx, itemID, expectedRevision, decision, c.now())
}

func (c *Coordinator) PrepareUpdate(ctx context.Context, id string, expectedRevision uint64, content *string, pinned *bool) (management.Mutation, error) {
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return management.Mutation{}, errors.Join(ErrInvalidInput, err)
	}
	if content != nil {
		canonical, err := domain.NormalizeContent(*content)
		if err != nil {
			return management.Mutation{}, errors.Join(ErrInvalidInput, err)
		}
		content = &canonical
	}
	return c.store.PrepareUpdate(ctx, itemID, expectedRevision, content, pinned, c.now())
}

func (c *Coordinator) PrepareDelete(ctx context.Context, id string, expectedRevision uint64) (management.Mutation, error) {
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return management.Mutation{}, errors.Join(ErrInvalidInput, err)
	}
	return c.store.PrepareDelete(ctx, itemID, expectedRevision)
}

func (c *Coordinator) PrepareAdd(ctx context.Context, scope domain.Scope, cwd, content string) (management.Mutation, error) {
	project, err := c.project(scope, cwd)
	if err != nil {
		return management.Mutation{}, err
	}
	content, err = domain.NormalizeContent(content)
	if err != nil {
		return management.Mutation{}, errors.Join(ErrInvalidInput, err)
	}
	return c.store.PrepareAdd(ctx, scope, project, content, c.now())
}

func (c *Coordinator) project(scope domain.Scope, cwd string) (string, error) {
	if err := scope.Validate(); err != nil {
		return "", errors.Join(ErrInvalidInput, err)
	}
	if scope == domain.ScopeUser {
		return "", nil
	}
	project, err := c.roots.ResolveExistingDir(cwd)
	if err != nil {
		return "", errors.Join(ErrWorkspaceUnavailable, err)
	}
	if err := domain.ValidateTarget(scope, project); err != nil {
		return "", err
	}
	return project, nil
}
