package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
)

type Schedules struct {
	contracts     rpc.Contracts
	installations plugins.Store
	pool          *mcpconnection.Pool
	publish       invalidation.Publish
	commands      *CommandCalls
}

func NewSchedules(store plugins.Store, pool *mcpconnection.Pool, commands *CommandCalls, publish invalidation.Publish) (*Schedules, error) {
	if dependency.Missing(store) || pool == nil {
		return nil, errors.New("schedules: installation store and connections are required")
	}
	if commands == nil {
		return nil, errors.New("schedules: command routing is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	return &Schedules{contracts: contracts, installations: store, pool: pool, publish: publish, commands: commands}, nil
}
func (s *Schedules) ListPage(ctx context.Context, cursor string, limit pagination.RequestedLimit) (pagination.Page[schedule.Schedule], error) {
	_, session, err := serviceSession(ctx, s.installations, s.commands.selections, s.pool, plugin.SchedulesService)
	if err != nil {
		return pagination.Page[schedule.Schedule]{}, err
	}
	page, err := s.contracts.List.Call(ctx, session, rpc.ListInput{Cursor: cursor, Limit: limit.Explicit()})
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
	value, err := invokeCommand[schedule.Draft, schedule.Snapshot](ctx, s.commands, plugin.SchedulesService, rpc.Create, input, func(value schedule.Snapshot) error { _, err := schedule.Restore(value); return err })
	if err != nil {
		s.publish.Notify(invalidation.ForSchedules())
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	saved, err := schedule.Restore(value.value)
	if err == nil {
		s.publish.Notify(invalidation.ForSchedules(saved.ID()))
	} else {
		s.publish.Notify(invalidation.ForSchedules())
	}
	return saved, err
}
func (s *Schedules) Update(ctx context.Context, input app.UpdateCommand) (schedule.Schedule, error) {
	value, err := invokeCommand[rpc.UpdateInput, schedule.Snapshot](ctx, s.commands, plugin.SchedulesService, rpc.Update, rpc.UpdateInput{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Title: input.Patch.Title, Instructions: input.Patch.Instructions, CWD: input.Patch.CWD, Cron: input.Patch.Cron, Enabled: input.Patch.Enabled, ModelSelection: rpc.EncodeModelPatch(input.Patch.ModelSelection)}, func(value schedule.Snapshot) error { _, err := schedule.Restore(value); return err })
	if err != nil {
		s.publish.Notify(invalidation.ForSchedules(input.ID))
		return schedule.Schedule{}, rpc.DecodeError(err)
	}
	saved, err := schedule.Restore(value.value)
	if err == nil {
		s.publish.Notify(invalidation.ForSchedules(saved.ID()))
	} else {
		s.publish.Notify(invalidation.ForSchedules())
	}
	return saved, err
}
func (s *Schedules) Delete(ctx context.Context, id string) error {
	_, err := invokeCommand[rpc.ItemInput, rpc.Ack](ctx, s.commands, plugin.SchedulesService, rpc.Delete, rpc.ItemInput{ID: id}, nil)
	s.publish.Notify(invalidation.ForSchedules(id))
	return rpc.DecodeError(err)
}
