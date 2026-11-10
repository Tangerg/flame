package management

import (
	"context"

	domain "github.com/Tangerg/flame/plugins/memory/internal/domain"
)

// Mutation retains a decided item and its persistence operation. Preparation
// and application share the caller's write transaction, so business decisions
// cannot change after the receipt's result has been captured.
type Mutation struct {
	item    domain.Item
	persist func(context.Context) error
}

func New(item domain.Item, persist func(context.Context) error) Mutation {
	return Mutation{item: item, persist: persist}
}

func (m Mutation) Item() domain.Item { return m.item }
func (m Mutation) Changed() bool     { return m.persist != nil }

func (m Mutation) Apply(ctx context.Context) error {
	if m.persist == nil {
		return nil
	}
	return m.persist(ctx)
}
