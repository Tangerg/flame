package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type Schedules struct {
	installations plugins.Store
	pool          *mcpconnection.Pool
	publish       invalidation.Publish
}

func NewSchedules(store plugins.Store, pool *mcpconnection.Pool, publish invalidation.Publish) (*Schedules, error) {
	if dependency.Missing(store) || pool == nil {
		return nil, errors.New("schedules: installation store and connections are required")
	}
	return &Schedules{installations: store, pool: pool, publish: publish}, nil
}
func (s *Schedules) ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[schedule.Schedule], error) {
	_, session, err := serviceSession(ctx, s.installations, s.pool, plugin.SchedulesService)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	page, err := pluginsdk.Call[rpc.ListInput, rpc.Page](ctx, session, rpc.List, rpc.ListInput{Cursor: cursor, Limit: limit.Explicit()})
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, rpc.DecodeError(err)
	}
	values := make([]schedule.Schedule, len(page.Rows))
	for i, row := range page.Rows {
		value, err := schedule.Restore(row)
		if err != nil {
			return pagination.Page[schedule.Schedule]{}, err
		}
		values[i] = value
	}
	return pagination.Page[schedule.Schedule]{Rows: values, NextCursor: page.NextCursor}, nil
}
func (s *Schedules) Create(ctx context.Context, input schedule.Draft) (schedule.Schedule, error) {
	_, session, err := serviceSession(ctx, s.installations, s.pool, plugin.SchedulesService)
	if err != nil {
		return schedule.Schedule{}, err
	}
	value, err := pluginsdk.CallCommand[schedule.Draft, schedule.Snapshot](ctx, session, rpc.Create, operation.Identity(ctx), input)
	if err != nil {
		s.publish.Notify(invalidation.ForSchedules())
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	saved, err := schedule.Restore(value)
	if err == nil {
		s.publish.Notify(invalidation.ForSchedules(saved.ID()))
	} else {
		s.publish.Notify(invalidation.ForSchedules())
	}
	return saved, err
}
func (s *Schedules) Update(ctx context.Context, input app.UpdateCommand) (schedule.Schedule, error) {
	_, session, err := serviceSession(ctx, s.installations, s.pool, plugin.SchedulesService)
	if err != nil {
		return schedule.Schedule{}, err
	}
	value, err := pluginsdk.CallCommand[rpc.UpdateInput, schedule.Snapshot](ctx, session, rpc.Update, operation.Identity(ctx), rpc.UpdateInput{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Title: input.Patch.Title, Instructions: input.Patch.Instructions, CWD: input.Patch.CWD, Cron: input.Patch.Cron, Enabled: input.Patch.Enabled, ModelSelection: input.Patch.ModelSelection})
	if err != nil {
		s.publish.Notify(invalidation.ForSchedules(input.ID))
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	saved, err := schedule.Restore(value)
	if err == nil {
		s.publish.Notify(invalidation.ForSchedules(saved.ID()))
	} else {
		s.publish.Notify(invalidation.ForSchedules())
	}
	return saved, err
}
func (s *Schedules) Delete(ctx context.Context, id string) error {
	_, session, err := serviceSession(ctx, s.installations, s.pool, plugin.SchedulesService)
	if err != nil {
		return err
	}
	_, err = pluginsdk.CallCommand[rpc.ItemInput, rpc.Ack](ctx, session, rpc.Delete, operation.Identity(ctx), rpc.ItemInput{ID: id})
	s.publish.Notify(invalidation.ForSchedules(id))
	return rpc.DecodeError(err)
}
