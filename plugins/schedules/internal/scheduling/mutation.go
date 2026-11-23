package scheduling

import (
	"context"

	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
)

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
