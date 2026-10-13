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
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type Schedules struct {
	installations plugins.Store
	pool          *mcpconnection.Pool
	publish       invalidation.Publish
}

func (s *Schedules) OwnsCreateSource(ctx context.Context, ref tool.Ref, release fingerprint.Digest) (bool, error) {
	server, remote, mcp := ref.MCP()
	if !mcp || remote.String() != "create_schedule" {
		return false, nil
	}
	binding, found, err := plugins.ServiceOwner(ctx, s.installations, plugin.SchedulesService)
	return found && binding.ServerID == server && binding.Digest == release, err
}

func (s *Schedules) OriginalCreateResult(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest, callID, arguments string) (*chat.ToolOutput, error) {
	session, err := s.originalCreateSession(ctx, ref, release, authority)
	if err != nil {
		return nil, err
	}
	result, err := managedmcp.Call[rpc.OriginalCreateInput, rpc.OriginalCreateOutput](ctx, session, rpc.OriginalCreateResult, rpc.OriginalCreateInput{CallID: callID, Arguments: arguments})
	if err != nil {
		return nil, rpc.DecodeError(err)
	}
	if result.Output != nil {
		if err := result.Output.Validate(); err != nil {
			return nil, err
		}
	}
	return result.Output, nil
}

func (s *Schedules) AwaitOriginalCreateSource(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest) error {
	_, err := s.originalCreateSession(ctx, ref, release, authority)
	return err
}

func (s *Schedules) originalCreateSession(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest) (*sdk.ClientSession, error) {
	owned, err := s.OwnsCreateSource(ctx, ref, release)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, plugin.ErrUnavailable
	}
	server, _, _ := ref.MCP()
	session, err := s.pool.BoundServiceSession(ctx, server, authority)
	if err != nil {
		return nil, err
	}
	return session, nil
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
	page, err := managedmcp.Call[rpc.ListInput, rpc.Page](ctx, session, rpc.List, rpc.ListInput{Cursor: cursor, Limit: limit.Explicit()})
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
	value, err := managedmcp.CallCommand[schedule.Draft, schedule.Snapshot](ctx, session, rpc.Create, operation.Identity(ctx), input)
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
	value, err := managedmcp.CallCommand[rpc.UpdateInput, schedule.Snapshot](ctx, session, rpc.Update, operation.Identity(ctx), rpc.UpdateInput{ID: input.ID, ExpectedRevision: input.ExpectedRevision, Title: input.Patch.Title, Instructions: input.Patch.Instructions, CWD: input.Patch.CWD, Cron: input.Patch.Cron, Enabled: input.Patch.Enabled, ModelSelection: input.Patch.ModelSelection})
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
	_, err = managedmcp.CallCommand[rpc.ItemInput, rpc.Ack](ctx, session, rpc.Delete, operation.Identity(ctx), rpc.ItemInput{ID: id})
	s.publish.Notify(invalidation.ForSchedules(id))
	return rpc.DecodeError(err)
}
