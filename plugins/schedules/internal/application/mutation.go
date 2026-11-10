package schedules

import (
	"context"

	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

// ScheduleMutation carries the exact state whose reply can be admitted before
// its persistence operation joins the caller's transaction.
type ScheduleMutation struct {
	state   schedule.Schedule
	persist func(context.Context) error
}

func (m ScheduleMutation) State() schedule.Schedule { return m.state }
func (m ScheduleMutation) Apply(ctx context.Context) error {
	if m.persist == nil {
		return nil
	}
	return m.persist(ctx)
}
