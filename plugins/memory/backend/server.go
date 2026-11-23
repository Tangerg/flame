package main

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	"github.com/Tangerg/flame/plugins/memory/internal/coordination"

	framesqlite "github.com/Tangerg/frame/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type store interface {
	coordination.Store
	coordination.CurationStore
	coordination.ReadStore
}
type capturedHistory struct{ conversation rpc.Conversation }

func (h capturedHistory) Read(context.Context, identity.SessionID) (coordination.Conversation, error) {
	return coordination.Conversation{MessageCount: h.conversation.MessageCount, Transcript: h.conversation.Transcript}, nil
}

func registerServer(server *sdk.Server, db framesqlite.Database, store store, roots coordination.RootResolver) error {
	contracts, err := rpc.NewContracts()
	if err != nil {
		return err
	}
	backend, err := pluginmcp.NewBackend(server)
	if err != nil {
		return err
	}
	review, err := coordination.New(coordination.Config{Store: store, Roots: roots})
	if err != nil {
		return err
	}
	read, err := coordination.NewReadModel(store, nil)
	if err != nil {
		return err
	}
	searchTool, err := newAgentMemorySearch(toolSearch{store: store, roots: roots})
	if err != nil {
		return err
	}
	tools, err := buildMemoryTools(review, db)
	if err != nil {
		return err
	}
	if err := pluginsdk.RegisterTools(server, append(tools, searchTool)...); err != nil {
		return err
	}
	toolReceipts, err := pluginsdk.ToolReceiptQuery()
	if err != nil {
		return err
	}
	handlers := memoryServer{db: db, store: store, review: review, read: read}
	if err := errors.Join(
		pluginstorage.RegisterReceipts(backend, db),
		toolReceipts.RegisterServer(backend, classify, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginsdk.ToolReceipt, error) {
			return memoryToolReceipt(ctx, db, command.Input)
		}),
		contracts.List.RegisterServer(backend, classify, handlers.list),
		registerManagementCommands(backend, db, contracts, review),
		contracts.Items.RegisterServer(backend, classify, handlers.items),
		contracts.Search.RegisterServer(backend, classify, handlers.search),
		contracts.Maintain.RegisterServer(backend, classify, handlers.maintain),
	); err != nil {
		return err
	}
	if err := backend.BindCapability("memory", contracts.Capability); err != nil {
		return err
	}
	return backend.Publish()
}

type memoryServer struct {
	db     framesqlite.Database
	store  store
	review *coordination.Coordinator
	read   *coordination.ReadModel
}

func (m memoryServer) list(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[rpc.ListInput]) (rpc.Page, error) {
	input := command.Input
	limit, err := pagination.ParseLimit(input.Limit)
	if err != nil {
		return rpc.Page{}, err
	}
	page, err := m.review.List(ctx, coordination.ListInput{Scope: input.Scope, CWD: input.CWD, Cursor: input.Cursor, Limit: limit})
	return rpc.Page{Rows: rpc.NewItems(page.Rows), NextCursor: page.NextCursor, Capacity: agentmemory.MaxVisiblePerTarget}, err
}

func (m memoryServer) items(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[rpc.ItemsInput]) ([]rpc.Item, error) {
	items, err := m.read.Items(ctx, command.Input.Scope, command.Input.Project)
	return rpc.NewItems(items), err
}

func (m memoryServer) search(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[rpc.SearchInput]) ([]rpc.Item, error) {
	input := command.Input
	var resolve func(context.Context) (coordination.Embedder, error)
	if input.EmbeddingSpace != "" {
		grant, err := pluginmcp.CallbackFromContext(ctx)
		if err != nil {
			return nil, err
		}
		resolve = func(context.Context) (coordination.Embedder, error) {
			return remoteModels{grant: grant, space: input.EmbeddingSpace}, nil
		}
	}
	read, err := coordination.NewReadModel(m.store, resolve)
	if err != nil {
		return nil, err
	}
	items, err := read.Search(ctx, input.Project, input.Query, input.TopK)
	return rpc.NewItems(items), err
}

func (m memoryServer) maintain(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[rpc.MaintenanceInput]) (rpc.MaintenanceResult, error) {
	input := command.Input
	admitted, err := command.RequestInput()
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	var grant pluginmcp.CallbackRef
	prepared, original, err := pluginstorage.AdmitCommand(ctx, m.db, rpc.Maintain, string(command.ID), admitted, func(ctx context.Context) error {
		if !input.Compacted {
			return nil
		}
		var err error
		grant, err = pluginmcp.CallbackFromContext(ctx)
		return err
	})
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	if prepared == nil {
		return pluginmcp.DecodeReply[rpc.MaintenanceResult](original)
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
	curation, err := coordination.NewCuration(maintenancePublication{CurationStore: m.store, db: m.db, command: prepared})
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	consolidator, err := coordination.NewMemoryConsolidator(capturedHistory{input.Conversation}, curation, remoteModels{grant: grant})
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	sessionID, err := identity.ParseOptionalSession(input.SessionID)
	var published bool
	if err == nil {
		published, err = consolidator.Consolidate(ctx, sessionID, input.Project)
	}
	if err != nil {
		return pluginstorage.CompleteExternalCommand(ctx, prepared, classify, func(context.Context) (pluginstorage.CommandPlan[rpc.MaintenanceResult], error) {
			return pluginstorage.CommandPlan[rpc.MaintenanceResult]{}, err
		})
	}
	reply, err := maintenanceReply(published)
	if err != nil {
		return rpc.MaintenanceResult{}, err
	}
	return rpc.MaintenanceResult{Published: published}, prepared.Complete(ctx, reply)
}

type toolSearch struct {
	store store
	roots coordination.RootResolver
}

func (s toolSearch) Search(ctx context.Context, project, query string, topK int) ([]agentmemory.Item, error) {
	project, err := s.roots.ResolveExistingDir(project)
	if err != nil {
		return nil, errors.Join(coordination.ErrWorkspaceUnavailable, err)
	}
	scope, err := pluginsdk.ToolContextFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var embed func(context.Context) (coordination.Embedder, error)
	if scope.EmbeddingSpace != "" {
		embed = func(context.Context) (coordination.Embedder, error) {
			grant, err := pluginmcp.CallbackFromContext(ctx)
			if err != nil {
				return nil, err
			}
			return remoteModels{grant: grant, space: scope.EmbeddingSpace}, nil
		}
	}
	read, err := coordination.NewReadModel(s.store, embed)
	if err != nil {
		return nil, err
	}
	return read.Search(ctx, project, query, topK)
}
