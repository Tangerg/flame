package agentmemory

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	domain "github.com/Tangerg/flame/runtime/internal/domain/workspace/agentmemory"
)

const managementPageNamespace = "agentMemory.management"
const managementPageLimit = 100

type ListInput struct {
	Scope  domain.Scope
	CWD    string
	Cursor string
	Limit  pagination.RequestedLimit
}

// The store's complete target is domain-bounded. Management owns ordering and
// continuation; a deleted anchor remains usable because the cursor carries its
// sort position rather than requiring that item to remain in storage.
func (c *Coordinator) List(ctx context.Context, input ListInput) (pagination.Page[domain.Item], error) {
	project, err := c.project(input.Scope, input.CWD)
	if err != nil {
		return pagination.Page[domain.Item]{}, err
	}
	limit, err := input.Limit.Resolve(managementPageLimit)
	if err != nil {
		return pagination.Page[domain.Item]{}, err
	}
	filters := []string{string(input.Scope), project}
	key, err := pagination.Decode(input.Cursor, managementPageNamespace, filters)
	if err != nil {
		return pagination.Page[domain.Item]{}, err
	}
	var anchor managementPosition
	if len(key) != 0 {
		anchor, err = decodeManagementPosition(key)
		if err != nil {
			return pagination.Page[domain.Item]{}, err
		}
	}
	items, err := c.store.List(ctx, input.Scope, project)
	if err != nil {
		return pagination.Page[domain.Item]{}, err
	}
	slices.SortFunc(items, func(a, b domain.Item) int { return compareManagementPosition(positionOf(a), positionOf(b)) })
	if len(key) != 0 {
		start := 0
		for start < len(items) && compareManagementPosition(positionOf(items[start]), anchor) <= 0 {
			start++
		}
		items = items[start:]
	}
	return pagination.PageOf(items, limit, managementPageNamespace, filters, func(item domain.Item) []string {
		return []string{string(item.Status()), strconv.FormatBool(item.Pinned()), item.UpdatedAt().UTC().Format(time.RFC3339Nano), item.ID().String()}
	})
}

type managementPosition struct {
	status  domain.Status
	pinned  bool
	updated time.Time
	id      string
}

func positionOf(item domain.Item) managementPosition {
	return managementPosition{status: item.Status(), pinned: item.Pinned(), updated: item.UpdatedAt(), id: item.ID().String()}
}

func decodeManagementPosition(key []string) (managementPosition, error) {
	if len(key) != 4 || (key[0] != string(domain.StatusActive) && key[0] != string(domain.StatusPending)) {
		return managementPosition{}, pagination.ErrInvalidCursor
	}
	pinned, err := strconv.ParseBool(key[1])
	if err != nil || strconv.FormatBool(pinned) != key[1] {
		return managementPosition{}, pagination.ErrInvalidCursor
	}
	updated, err := time.Parse(time.RFC3339Nano, key[2])
	if err != nil {
		return managementPosition{}, pagination.ErrInvalidCursor
	}
	id, err := domain.ParseItemID(key[3])
	if err != nil {
		return managementPosition{}, pagination.ErrInvalidCursor
	}
	return managementPosition{status: domain.Status(key[0]), pinned: pinned, updated: updated, id: id.String()}, nil
}

func compareManagementPosition(a, b managementPosition) int {
	if a.status != b.status {
		if a.status == domain.StatusPending {
			return -1
		}
		return 1
	}
	return compareActivePosition(a, b)
}

func compareActiveItems(a, b domain.Item) int {
	return compareActivePosition(positionOf(a), positionOf(b))
}

func compareActivePosition(a, b managementPosition) int {
	if a.pinned != b.pinned {
		if a.pinned {
			return -1
		}
		return 1
	}
	if order := b.updated.Compare(a.updated); order != 0 {
		return order
	}
	return cmp.Compare(b.id, a.id)
}
