package transport

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	app "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/maintenance"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/tools"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Store interface {
	app.Store
	app.CurationStore
	app.ReadStore
}
type capturedHistory struct{ conversation rpc.Conversation }

func (h capturedHistory) Read(context.Context, string) (maintenance.Conversation, error) {
	return maintenance.Conversation{MessageCount: h.conversation.MessageCount, Transcript: h.conversation.Transcript}, nil
}
func snapshots(items []memory.Item) []memory.ItemSnapshot {
	result := make([]memory.ItemSnapshot, len(items))
	for i, item := range items {
		result[i] = item.Snapshot()
	}
	return result
}

func RegisterServer(server *sdk.Server, db *sql.DB, store Store, roots app.RootResolver) error {
	backend, err := pluginsdk.NewBackend(server)
	if err != nil {
		return err
	}

	review, err := app.New(app.Config{Store: store, Roots: roots})
	if err != nil {
		return err
	}
	read, err := app.NewReadModel(store, nil)
	if err != nil {
		return err
	}
	searchTool, err := tools.NewAgentMemorySearch(toolSearch{store: store})
	if err != nil {
		return err
	}
	if err := pluginsdk.RegisterTools(server, searchTool); err != nil {
		return err
	}
	if err := errors.Join(
		managedmcp.RegisterReceipts(backend, db),
		pluginsdk.RegisterServer(backend, rpc.List, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ListInput) (rpc.Page, error) {
			limit := pagination.DefaultLimit()
			if input.Limit != nil {
				var err error
				limit, err = pagination.NewLimit(*input.Limit)
				if err != nil {
					return rpc.Page{}, err
				}
			}
			page, err := review.List(ctx, app.ListInput{Scope: input.Scope, CWD: input.CWD, Cursor: input.Cursor, Limit: limit})
			return rpc.Page{Rows: snapshots(page.Rows), NextCursor: page.NextCursor}, err
		}),
		managedmcp.RegisterCommand(backend, db, rpc.Add, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.AddInput) (rpc.AddResult, error) {
			changed := false
			owner, err := app.New(app.Config{Store: store, Roots: roots, Invalidations: func(invalidation.Notice) { changed = true }})
			if err != nil {
				return rpc.AddResult{}, err
			}
			item, err := owner.Add(ctx, input.Scope, input.CWD, input.Content)
			return rpc.AddResult{Item: item.Snapshot(), Changed: changed}, err
		}),
		managedmcp.RegisterCommand(backend, db, rpc.Update, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.UpdateInput) (memory.ItemSnapshot, error) {
			item, err := review.Update(ctx, input.ID, input.Content, input.Pinned)
			return item.Snapshot(), err
		}),
		managedmcp.RegisterCommand(backend, db, rpc.Delete, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemInput) (rpc.Ack, error) {
			return rpc.Ack{}, review.Delete(ctx, input.ID)
		}),
		managedmcp.RegisterCommand(backend, db, rpc.Review, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ReviewInput) (rpc.Ack, error) {
			return rpc.Ack{}, review.Review(ctx, input.ID, input.Decision)
		}),
		pluginsdk.RegisterServer(backend, rpc.Items, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemsInput) ([]memory.ItemSnapshot, error) {
			items, err := read.Items(ctx, input.Scope, input.Project)
			return snapshots(items), err
		}),
		pluginsdk.RegisterServer(backend, rpc.Search, rpc.Classify, func(ctx context.Context, session *sdk.ServerSession, input rpc.SearchInput) ([]memory.ItemSnapshot, error) {
			var resolve func(context.Context) (app.Embedder, error)
			if input.EmbeddingSpace != "" {
				resolve = func(context.Context) (app.Embedder, error) {
					return rpc.RemoteModels{Grant: input.Grant, Space: input.EmbeddingSpace}, nil
				}
			}
			read, err := app.NewReadModel(store, resolve)
			if err != nil {
				return nil, err
			}
			items, err := read.Search(ctx, input.Project, input.Query, input.TopK)
			return snapshots(items), err
		}),
		pluginsdk.RegisterCommand(backend, rpc.Maintain, rpc.Classify, func(ctx context.Context, session *sdk.ServerSession, command pluginsdk.Command[rpc.MaintenanceInput]) (rpc.MaintenanceResult, error) {
			input := command.Input
			admitted, err := command.RequestInput()
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			prepared, original, err := managedmcp.PrepareCommand(ctx, db, rpc.Maintain, command.ID, admitted)
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			if prepared == nil {
				return pluginsdk.DecodeReply[rpc.MaintenanceResult](original)
			}
			if !input.Compacted {
				reply, err := maintenanceReply(false)
				if err != nil {
					return rpc.MaintenanceResult{}, err
				}
				return rpc.MaintenanceResult{}, prepared.Complete(ctx, reply)
			}
			published := false
			curation, err := app.NewCuration(app.CurationConfig{Store: maintenancePublication{CurationStore: store, db: db, command: prepared}, Invalidations: func(invalidation.Notice) { published = true }})
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			consolidator, err := maintenance.NewMemoryConsolidator(capturedHistory{input.Conversation}, curation, rpc.RemoteModels{Grant: input.Grant}, maintenance.MemoryCurationPolicyValues{})
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			err = consolidator.Consolidate(ctx, input.SessionID, input.Project)
			if err != nil {
				return rpc.MaintenanceResult{Published: published}, err
			}
			reply, err := maintenanceReply(published)
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			return rpc.MaintenanceResult{Published: published}, prepared.Complete(ctx, reply)
		}),
	); err != nil {
		return err
	}
	return backend.Publish()
}

type toolSearch struct{ store Store }

func (s toolSearch) Search(ctx context.Context, project, query string, topK int) ([]memory.Item, error) {
	scope, err := rpc.MemoryScope(ctx)
	if err != nil {
		return nil, err
	}
	var embed func(context.Context) (app.Embedder, error)
	if scope.EmbeddingSpace != "" {
		embed = func(context.Context) (app.Embedder, error) {
			return rpc.RemoteModels{Grant: scope.Grant, Space: scope.EmbeddingSpace}, nil
		}
	}
	read, err := app.NewReadModel(s.store, embed)
	if err != nil {
		return nil, err
	}
	return read.Search(ctx, project, query, topK)
}
