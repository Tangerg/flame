package transport

import (
	"context"
	"database/sql"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	app "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/maintenance"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/tools"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	pluginstorage "github.com/Tangerg/flame/runtime/plugin/sqlite"
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
	contracts, err := rpc.NewContracts()
	if err != nil {
		return err
	}
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
		pluginstorage.RegisterReceipts(backend, db),
		contracts.List.RegisterServer(backend, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ListInput]) (rpc.Page, error) {
			input := command.Input
			limit := pagination.DefaultLimit()
			if input.Limit != nil {
				var err error
				limit, err = pagination.NewLimit(*input.Limit)
				if err != nil {
					return rpc.Page{}, err
				}
			}
			page, err := review.List(ctx, app.ListInput{Scope: input.Scope, CWD: input.CWD, Cursor: input.Cursor, Limit: limit})
			return rpc.Page{Rows: snapshots(page.Rows), NextCursor: page.NextCursor, Capacity: memory.MaxVisiblePerTarget}, err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Add, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.AddInput) (rpc.AddResult, error) {
			changed := false
			owner, err := app.New(app.Config{Store: store, Roots: roots, OnChange: func() { changed = true }})
			if err != nil {
				return rpc.AddResult{}, err
			}
			item, err := owner.Add(ctx, input.Scope, input.CWD, input.Content)
			return rpc.AddResult{Item: item.Snapshot(), Changed: changed}, err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Update, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.UpdateInput) (memory.ItemSnapshot, error) {
			item, err := review.Update(ctx, input.ID, input.Content, input.Pinned)
			return item.Snapshot(), err
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Delete, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ItemInput) (rpc.Ack, error) {
			return rpc.Ack{}, review.Delete(ctx, input.ID)
		}),
		pluginstorage.RegisterCommand(backend, db, contracts.Review, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, input rpc.ReviewInput) (rpc.Ack, error) {
			return rpc.Ack{}, review.Review(ctx, input.ID, input.Decision)
		}),
		contracts.Items.RegisterServer(backend, rpc.Classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginsdk.Command[rpc.ItemsInput]) ([]memory.ItemSnapshot, error) {
			input := command.Input
			items, err := read.Items(ctx, input.Scope, input.Project)
			return snapshots(items), err
		}),
		contracts.Search.RegisterServer(backend, rpc.Classify, func(ctx context.Context, session *sdk.ServerSession, command pluginsdk.Command[rpc.SearchInput]) ([]memory.ItemSnapshot, error) {
			input := command.Input
			var resolve func(context.Context) (app.Embedder, error)
			if input.EmbeddingSpace != "" {
				grant, err := pluginsdk.CallbackFromContext(ctx)
				if err != nil {
					return nil, err
				}
				resolve = func(context.Context) (app.Embedder, error) {
					return rpc.RemoteModels{Grant: grant, Space: input.EmbeddingSpace}, nil
				}
			}
			read, err := app.NewReadModel(store, resolve)
			if err != nil {
				return nil, err
			}
			items, err := read.Search(ctx, input.Project, input.Query, input.TopK)
			return snapshots(items), err
		}),
		contracts.Maintain.RegisterServer(backend, rpc.Classify, func(ctx context.Context, session *sdk.ServerSession, command pluginsdk.Command[rpc.MaintenanceInput]) (rpc.MaintenanceResult, error) {
			input := command.Input
			admitted, err := command.RequestInput()
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			var grant pluginsdk.CallbackRef
			prepared, original, err := pluginstorage.AdmitCommand(ctx, db, rpc.Maintain, command.ID, admitted, func(ctx context.Context) error {
				if !input.Compacted {
					return nil
				}
				var err error
				grant, err = pluginsdk.CallbackFromContext(ctx)
				return err
			})
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			if prepared == nil {
				return pluginsdk.DecodeReply[rpc.MaintenanceResult](original)
			}
			if !prepared.NewlyAdmitted() {
				return rpc.MaintenanceResult{}, pluginstorage.ErrCommandUnconfirmed
			}
			if !input.Compacted {
				reply, err := maintenanceReply(false)
				if err != nil {
					return rpc.MaintenanceResult{}, err
				}
				return rpc.MaintenanceResult{}, prepared.Complete(ctx, reply)
			}
			published := false
			curation, err := app.NewCuration(app.CurationConfig{Store: maintenancePublication{CurationStore: store, db: db, command: prepared}, OnChange: func() { published = true }})
			if err != nil {
				return rpc.MaintenanceResult{}, err
			}
			consolidator, err := maintenance.NewMemoryConsolidator(capturedHistory{input.Conversation}, curation, rpc.RemoteModels{Grant: grant}, maintenance.MemoryCurationPolicyValues{})
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
	if err := backend.BindCapability("memory", contracts.Capability); err != nil {
		return err
	}
	return backend.Publish()
}

type toolSearch struct{ store Store }

func (s toolSearch) Search(ctx context.Context, project, query string, topK int) ([]memory.Item, error) {
	scope, err := pluginsdk.ToolContextFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var embed func(context.Context) (app.Embedder, error)
	if scope.EmbeddingSpace != "" {
		embed = func(context.Context) (app.Embedder, error) {
			grant, err := pluginsdk.CallbackFromContext(ctx)
			if err != nil {
				return nil, err
			}
			return rpc.RemoteModels{Grant: grant, Space: scope.EmbeddingSpace}, nil
		}
	}
	read, err := app.NewReadModel(s.store, embed)
	if err != nil {
		return nil, err
	}
	return read.Search(ctx, project, query, topK)
}
