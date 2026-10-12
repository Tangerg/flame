package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	modeltext "github.com/Tangerg/flame/runtime/internal/adapter/run/modeltext"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	app "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	"github.com/Tangerg/scope/core/chat"
)

type Memory struct {
	installations plugins.Store
	pool          *mcpconnection.Pool
	models        *managedmcp.Models
	utility       models.AuxiliaryResolver
	embedder      func(context.Context) (app.Embedder, error)
	history       ConversationReader
	publish       invalidation.Publish
}

type ConversationReader interface {
	Read(context.Context, string) ([]chat.Message, error)
}
type MemoryConfig struct {
	Installations plugins.Store
	Connections   *mcpconnection.Pool
	Models        *managedmcp.Models
	Utility       models.AuxiliaryResolver
	Embeddings    func(context.Context) (app.Embedder, error)
	Conversation  ConversationReader
	Invalidations invalidation.Publish
}

func NewMemory(config MemoryConfig) (*Memory, error) {
	for _, required := range []struct {
		name  string
		value any
	}{{"installation store", config.Installations}, {"connections", config.Connections}, {"models", config.Models}, {"utility resolver", config.Utility}, {"embedding resolver", config.Embeddings}, {"conversation reader", config.Conversation}} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("memory: %s is required", required.name)
		}
	}
	return &Memory{installations: config.Installations, pool: config.Connections, models: config.Models, utility: config.Utility, embedder: config.Embeddings, history: config.Conversation, publish: config.Invalidations}, nil
}
func (m *Memory) List(ctx context.Context, input app.ListInput) (pagination.Page[memory.Item], error) {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if err != nil {
		return pagination.Page[memory.Item]{}, err
	}
	value, err := pluginsdk.Call[rpc.ListInput, rpc.Page](ctx, session, rpc.List, rpc.ListInput{Scope: input.Scope, CWD: input.CWD, Cursor: input.Cursor, Limit: input.Limit.Explicit()})
	if err != nil {
		return pagination.Page[memory.Item]{}, rpc.DecodeError(err)
	}
	items, err := restoreItems(value.Rows)
	return pagination.Page[memory.Item]{Rows: items, NextCursor: value.NextCursor}, err
}
func (m *Memory) Add(ctx context.Context, scope memory.Scope, cwd, content string) (memory.Item, error) {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if err != nil {
		return memory.Item{}, err
	}
	value, err := pluginsdk.CallCommand[rpc.AddInput, rpc.AddResult](ctx, session, rpc.Add, operation.Identity(ctx), rpc.AddInput{Scope: scope, CWD: cwd, Content: content})
	if err != nil {
		m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
		return memory.Item{}, rpc.DecodeError(err)
	}
	item, err := memory.RestoreItem(value.Item)
	if value.Changed {
		m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	}
	return item, err
}
func (m *Memory) Update(ctx context.Context, id string, content *string, pinned *bool) (memory.Item, error) {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if err != nil {
		return memory.Item{}, err
	}
	value, err := pluginsdk.CallCommand[rpc.UpdateInput, memory.ItemSnapshot](ctx, session, rpc.Update, operation.Identity(ctx), rpc.UpdateInput{ID: id, Content: content, Pinned: pinned})
	if err != nil {
		m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
		return memory.Item{}, rpc.DecodeError(err)
	}
	item, err := memory.RestoreItem(value)
	m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return item, err
}
func (m *Memory) Review(ctx context.Context, id string, decision memory.ReviewDecision) error {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if err != nil {
		return err
	}
	_, err = pluginsdk.CallCommand[rpc.ReviewInput, rpc.Ack](ctx, session, rpc.Review, operation.Identity(ctx), rpc.ReviewInput{ID: id, Decision: decision})
	m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return rpc.DecodeError(err)
}
func (m *Memory) Delete(ctx context.Context, id string) error {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if err != nil {
		return err
	}
	_, err = pluginsdk.CallCommand[rpc.ItemInput, rpc.Ack](ctx, session, rpc.Delete, operation.Identity(ctx), rpc.ItemInput{ID: id})
	m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return rpc.DecodeError(err)
}
func (m *Memory) Items(ctx context.Context, scope memory.Scope, project string) ([]memory.Item, error) {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values, err := pluginsdk.Call[rpc.ItemsInput, []memory.ItemSnapshot](ctx, session, rpc.Items, rpc.ItemsInput{Scope: scope, Project: project})
	if err != nil {
		return nil, rpc.DecodeError(err)
	}
	items, err := restoreItems(values)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Status() != memory.StatusActive || item.Scope() != scope || item.Project() != project {
			return nil, errors.New("memory: backend returned items outside the requested active target")
		}
	}
	return items, nil
}
func (m *Memory) Search(ctx context.Context, project, query string, topK int) ([]memory.Item, error) {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	embedder, err := m.embedder(ctx)
	if err != nil {
		slog.WarnContext(ctx, "memory: resolve optional embedding role", "error", err)
		embedder = nil
	}
	grant, release, err := m.models.Grant(ctx, session, nil, embedder)
	if err != nil {
		return nil, err
	}
	defer release()
	space := ""
	if embedder != nil {
		space = embedder.ID()
	}
	values, err := pluginsdk.Call[rpc.SearchInput, []memory.ItemSnapshot](ctx, session, rpc.Search, rpc.SearchInput{Project: project, Query: query, TopK: topK, Grant: grant, EmbeddingSpace: space})
	if err != nil {
		return nil, rpc.DecodeError(err)
	}
	items, err := restoreItems(values)
	if err != nil {
		return nil, err
	}
	if len(items) > max(topK, 0) {
		return nil, errors.New("memory: backend exceeded requested search bound")
	}
	for _, item := range items {
		if item.Status() != memory.StatusActive || item.Scope() != memory.ScopeUser && (item.Scope() != memory.ScopeProject || item.Project() != project) {
			return nil, errors.New("memory: backend returned search outside the active target")
		}
	}
	return items, nil
}
func (m *Memory) MaintainMemory(ctx context.Context, input execution.RunMaintenanceInput) error {
	_, session, err := serviceSession(ctx, m.installations, m.pool, plugin.MemoryService)
	if errors.Is(err, errInactive) {
		return nil
	}
	if err != nil {
		return err
	}
	messages, err := m.history.Read(ctx, input.SessionID)
	if err != nil {
		return fmt.Errorf("memory: capture committed conversation: %w", err)
	}
	grant, release, err := m.models.Grant(ctx, session, func(ctx context.Context, prompt managedmcp.Prompt) (string, error) {
		return m.utility.Complete(ctx, models.AuxiliaryPrompt{Operation: prompt.Operation, SystemPrompt: prompt.SystemPrompt, UserPrompt: prompt.UserPrompt, MaxInputBytes: prompt.MaxInputBytes, MaxOutputTokens: prompt.MaxOutputTokens})
	}, nil)
	if err != nil {
		return err
	}
	defer release()
	result, err := pluginsdk.Call[rpc.MaintenanceInput, rpc.MaintenanceResult](ctx, session, rpc.Maintain, rpc.MaintenanceInput{SessionID: input.SessionID, Project: input.WorkspaceCWD, Compacted: input.DurableContextCompacted, Grant: grant, Conversation: rpc.Conversation{MessageCount: len(messages), Transcript: modeltext.RenderTranscript(messages)}})
	if err != nil || result.Published {
		m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	}
	return rpc.DecodeError(err)
}
func restoreItems(values []memory.ItemSnapshot) ([]memory.Item, error) {
	items := make([]memory.Item, len(values))
	for i, value := range values {
		item, err := memory.RestoreItem(value)
		if err != nil {
			return nil, err
		}
		items[i] = item
	}
	return items, nil
}
