package management

import (
	"context"

	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
)

type Mutation struct {
	item    agentmemory.Item
	persist func(context.Context) error
}

func New(item agentmemory.Item, persist func(context.Context) error) Mutation {
	return Mutation{item: item, persist: persist}
}

func (m Mutation) Item() agentmemory.Item { return m.item }
func (m Mutation) Changed() bool          { return m.persist != nil }

func (m Mutation) Apply(ctx context.Context) error {
	if m.persist == nil {
		return nil
	}
	return m.persist(ctx)
}
