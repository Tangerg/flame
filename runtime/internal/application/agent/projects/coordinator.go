package projects

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/project"
	"github.com/samber/lo"
)

type CatalogAnchor struct {
	UpdatedAt time.Time
	ID        identity.ProjectID
}

type CatalogRead struct {
	Archived bool
	After    *CatalogAnchor
	Limit    int
}

type Store interface {
	Insert(ctx context.Context, value project.Project) error
	Save(ctx context.Context, expectedRevision uint64, value project.Project) error
	Get(ctx context.Context, id identity.ProjectID) (project.Project, error)
	ListPage(ctx context.Context, read CatalogRead) ([]project.Project, error)
}

type Config struct {
	Store         Store
	NewID         func() identity.ProjectID
	Now           func() time.Time
	Invalidations invalidation.Publish
}

type Coordinator struct {
	store         Store
	newID         func() identity.ProjectID
	now           func() time.Time
	invalidations invalidation.Publish
}

func New(config Config) (*Coordinator, error) {
	if lo.IsNil(config.Store) || config.NewID == nil || config.Now == nil || config.Invalidations == nil {
		return nil, errors.New("projects: store, identity source, clock and invalidations are required")
	}
	return &Coordinator{store: config.Store, newID: config.NewID, now: config.Now, invalidations: config.Invalidations}, nil
}

func (c *Coordinator) Create(ctx context.Context, name, instructions string, complete operation.Reply[project.Project]) error {
	value, err := project.New(project.Draft{ID: c.newID(), Name: name, Instructions: instructions, CreatedAt: c.now()})
	if err != nil {
		return err
	}
	if err := complete(ctx, func(tx context.Context) (project.Project, error) {
		return value, c.store.Insert(tx, value)
	}); err != nil {
		return err
	}
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Projects})
	return nil
}

func (c *Coordinator) Update(ctx context.Context, id identity.ProjectID, expectedRevision uint64, patch project.Patch, complete operation.Reply[project.Project]) error {
	current, err := c.store.Get(ctx, id)
	if err != nil {
		return err
	}
	next, changed, err := current.Edit(expectedRevision, patch, c.now())
	if err != nil {
		return err
	}
	if err := complete(ctx, func(tx context.Context) (project.Project, error) {
		if !changed {
			return current, nil
		}
		return next, c.store.Save(tx, expectedRevision, next)
	}); err != nil {
		return err
	}
	if changed {
		c.invalidations.Notify(invalidation.Notice{Resource: invalidation.Projects})
	}
	return nil
}

func (c *Coordinator) Get(ctx context.Context, id identity.ProjectID) (project.Project, error) {
	return c.store.Get(ctx, id)
}

const (
	pageNamespace = "projects"
	pageLimit     = 100
)

func (c *Coordinator) ListPage(ctx context.Context, archived bool, cursor string, limit pagination.RequestedLimit) (pagination.Page[project.Project], error) {
	filters := []string{"archived", strconv.FormatBool(archived)}
	key, err := pagination.Decode(cursor, pageNamespace, filters)
	if err != nil {
		return pagination.Page[project.Project]{}, err
	}
	after, err := parseAnchor(key)
	if err != nil {
		return pagination.Page[project.Project]{}, err
	}
	size, err := limit.Resolve(pageLimit)
	if err != nil {
		return pagination.Page[project.Project]{}, err
	}
	values, err := c.store.ListPage(ctx, CatalogRead{Archived: archived, After: after, Limit: size + 1})
	if err != nil {
		return pagination.Page[project.Project]{}, err
	}
	return pagination.PageOf(values, size, pageNamespace, filters, func(value project.Project) []string {
		return []string{strconv.FormatInt(value.UpdatedAt().UnixNano(), 10), value.ID().String()}
	})
}

func parseAnchor(key []string) (*CatalogAnchor, error) {
	if len(key) == 0 {
		return nil, nil
	}
	if len(key) != 2 {
		return nil, pagination.ErrInvalidCursor
	}
	updatedAt, err := strconv.ParseInt(key[0], 10, 64)
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	id, err := identity.ParseProject(key[1])
	if err != nil {
		return nil, pagination.ErrInvalidCursor
	}
	return &CatalogAnchor{UpdatedAt: time.Unix(0, updatedAt).UTC(), ID: id}, nil
}
