package coordination

import (
	"context"
	"errors"
	"time"

	domain "github.com/Tangerg/flame/plugins/memory/internal/domain"
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
	Review(ctx context.Context, id domain.ItemID, expectedRevision uint64, decision domain.ReviewDecision, now time.Time) error
	Update(ctx context.Context, id domain.ItemID, expectedRevision uint64, content *string, pinned *bool, now time.Time) (domain.Item, error)
	Delete(ctx context.Context, id domain.ItemID, expectedRevision uint64) error
	Add(ctx context.Context, scope domain.Scope, project, content string, now time.Time) (item domain.Item, changed bool, err error)
}

type Config struct {
	Store Store
	Roots RootResolver
	Now   func() time.Time
	// The caller owns external publication after the current write-set commits.
	OnChange func()
}

type Coordinator struct {
	store    Store
	roots    RootResolver
	now      func() time.Time
	onChange func()
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
	return &Coordinator{store: cfg.Store, roots: cfg.Roots, now: now, onChange: cfg.OnChange}, nil
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

func (c *Coordinator) Review(ctx context.Context, id string, expectedRevision uint64, decision domain.ReviewDecision) error {
	if _, err := decision.Result(); err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	if err := c.store.Review(ctx, itemID, expectedRevision, decision, c.now()); err != nil {
		return err
	}
	if c.onChange != nil {
		c.onChange()
	}
	return nil
}

func (c *Coordinator) Update(ctx context.Context, id string, expectedRevision uint64, content *string, pinned *bool) (domain.Item, error) {
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return domain.Item{}, errors.Join(ErrInvalidInput, err)
	}
	if content != nil {
		canonical, err := domain.NormalizeContent(*content)
		if err != nil {
			return domain.Item{}, errors.Join(ErrInvalidInput, err)
		}
		content = &canonical
	}
	item, err := c.store.Update(ctx, itemID, expectedRevision, content, pinned, c.now())
	if err != nil {
		return domain.Item{}, err
	}
	if c.onChange != nil {
		c.onChange()
	}
	return item, nil
}

func (c *Coordinator) Delete(ctx context.Context, id string, expectedRevision uint64) error {
	itemID, err := domain.ParseItemID(id)
	if err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	if err := c.store.Delete(ctx, itemID, expectedRevision); err != nil {
		return err
	}
	if c.onChange != nil {
		c.onChange()
	}
	return nil
}

func (c *Coordinator) Add(ctx context.Context, scope domain.Scope, cwd, content string) (domain.Item, error) {
	project, err := c.project(scope, cwd)
	if err != nil {
		return domain.Item{}, err
	}
	content, err = domain.NormalizeContent(content)
	if err != nil {
		return domain.Item{}, errors.Join(ErrInvalidInput, err)
	}
	item, changed, err := c.store.Add(ctx, scope, project, content, c.now())
	if err != nil {
		return domain.Item{}, err
	}
	if changed {
		if c.onChange != nil {
			c.onChange()
		}
	}
	return item, nil
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
