package pluginservices

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	modeltext "github.com/Tangerg/flame/runtime/internal/adapter/run/modeltext"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	app "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	rpc "github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
)

type Memory struct {
	contracts     rpc.Contracts
	installations plugins.Store
	pool          *mcpconnection.Pool
	callbacks     *pluginsdk.Callbacks
	utility       models.AuxiliaryResolver
	embedder      func(context.Context) (app.Embedder, error)
	history       ConversationReader
	publish       invalidation.Publish
	commands      *CommandCalls
}

type ConversationReader interface {
	Read(context.Context, string) ([]chat.Message, error)
}
type MemoryConfig struct {
	Installations plugins.Store
	Connections   *mcpconnection.Pool
	Callbacks     *pluginsdk.Callbacks
	Utility       models.AuxiliaryResolver
	Embeddings    func(context.Context) (app.Embedder, error)
	Conversation  ConversationReader
	Invalidations invalidation.Publish
	Commands      *CommandCalls
}

func NewMemory(config MemoryConfig) (*Memory, error) {
	for _, required := range []struct {
		name  string
		value any
	}{{"installation store", config.Installations}, {"connections", config.Connections}, {"callbacks", config.Callbacks}, {"utility resolver", config.Utility}, {"embedding resolver", config.Embeddings}, {"conversation reader", config.Conversation}} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("memory: %s is required", required.name)
		}
	}
	if config.Commands == nil {
		return nil, errors.New("memory: command routing is required")
	}
	contracts, err := rpc.NewContracts()
	if err != nil {
		return nil, err
	}
	return &Memory{contracts: contracts, installations: config.Installations, pool: config.Connections, callbacks: config.Callbacks, utility: config.Utility, embedder: config.Embeddings, history: config.Conversation, publish: config.Invalidations, commands: config.Commands}, nil
}
func (m *Memory) List(ctx context.Context, input app.ListInput) (pagination.Page[memory.Item], error) {
	location, session, err := contributionSession(ctx, m.installations, m.commands.selections, m.pool, m.contracts.Capability)
	if err != nil {
		return pagination.Page[memory.Item]{}, err
	}
	value, err := query(ctx, location, session, m.contracts.List, rpc.ListInput{Scope: input.Scope, CWD: input.CWD, Cursor: input.Cursor, Limit: input.Limit.Explicit()}, sdk.ParamsBase{})
	if err != nil {
		return pagination.Page[memory.Item]{}, rpc.DecodeError(err)
	}
	items, err := restoreItems(value.Rows)
	return pagination.Page[memory.Item]{Rows: items, NextCursor: value.NextCursor}, err
}
func (m *Memory) Add(ctx context.Context, scope memory.Scope, cwd, content string) (memory.Item, error) {
	value, err := invokeCommand[rpc.AddInput, rpc.AddResult](ctx, m.commands, m.contracts.Capability, rpc.Add, rpc.AddInput{Scope: scope, CWD: cwd, Content: content}, func(value rpc.AddResult) error { _, err := memory.RestoreItem(value.Item); return err })
	if err != nil {
		m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
		return memory.Item{}, rpc.DecodeError(err)
	}
	item, err := memory.RestoreItem(value.value.Item)
	if value.value.Changed {
		m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	}
	return item, err
}
func (m *Memory) Update(ctx context.Context, id string, content *string, pinned *bool) (memory.Item, error) {
	value, err := invokeCommand[rpc.UpdateInput, memory.ItemSnapshot](ctx, m.commands, m.contracts.Capability, rpc.Update, rpc.UpdateInput{ID: id, Content: content, Pinned: pinned}, func(value memory.ItemSnapshot) error { _, err := memory.RestoreItem(value); return err })
	if err != nil {
		m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
		return memory.Item{}, rpc.DecodeError(err)
	}
	item, err := memory.RestoreItem(value.value)
	m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return item, err
}
func (m *Memory) Review(ctx context.Context, id string, decision memory.ReviewDecision) error {
	_, err := invokeCommand[rpc.ReviewInput, rpc.Ack](ctx, m.commands, m.contracts.Capability, rpc.Review, rpc.ReviewInput{ID: id, Decision: decision}, nil)
	m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return rpc.DecodeError(err)
}
func (m *Memory) Delete(ctx context.Context, id string) error {
	_, err := invokeCommand[rpc.ItemInput, rpc.Ack](ctx, m.commands, m.contracts.Capability, rpc.Delete, rpc.ItemInput{ID: id}, nil)
	m.publish.Notify(invalidation.Notice{Resource: invalidation.AgentMemory})
	return rpc.DecodeError(err)
}
func (m *Memory) Items(ctx context.Context, scope memory.Scope, project string) ([]memory.Item, error) {
	location, session, err := contributionSession(ctx, m.installations, m.commands.selections, m.pool, m.contracts.Capability)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values, err := query(ctx, location, session, m.contracts.Items, rpc.ItemsInput{Scope: scope, Project: project}, sdk.ParamsBase{})
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
func (m *Memory) Search(ctx context.Context, project, text string, topK int) ([]memory.Item, error) {
	location, session, err := contributionSession(ctx, m.installations, m.commands.selections, m.pool, m.contracts.Capability)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	embedder, err := m.embedder(ctx)
	if err != nil {
		return nil, err
	}
	grant, release, err := rpc.GrantModels(ctx, m.callbacks, session, nil, embedder)
	if err != nil {
		return nil, err
	}
	defer release()
	space := ""
	if embedder != nil {
		space = embedder.ID()
	}
	metadata, err := pluginsdk.CallbackMetadata(grant)
	if err != nil {
		return nil, err
	}
	values, err := query(ctx, location, session, m.contracts.Search, rpc.SearchInput{Project: project, Query: text, TopK: topK, EmbeddingSpace: space}, metadata)
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
	binding, _, err := contributionSession(ctx, m.installations, m.commands.selections, m.pool, m.contracts.Capability)
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
	result, err := invokeResolvedCommand[rpc.MaintenanceInput, rpc.MaintenanceResult](operation.Bind(ctx, "plugins.automatic", uuid.NewString()), m.commands, rpc.Maintain, rpc.MaintenanceInput{SessionID: input.SessionID, Project: input.WorkspaceCWD, Compacted: input.DurableContextCompacted, Conversation: rpc.Conversation{MessageCount: len(messages), Transcript: modeltext.RenderTranscript(messages)}}, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, func(ctx context.Context, _ plugins.ContributionBinding, original *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
		grant, release, err := rpc.GrantModels(ctx, m.callbacks, original, func(ctx context.Context, prompt rpc.Prompt) (string, error) {
			return m.utility.Complete(ctx, models.AuxiliaryPrompt{Operation: prompt.Operation, SystemPrompt: prompt.SystemPrompt, UserPrompt: prompt.UserPrompt, MaxInputBytes: prompt.MaxInputBytes, MaxOutputTokens: prompt.MaxOutputTokens})
		}, nil)
		if err != nil {
			return sdk.ParamsBase{}, nil, err
		}
		metadata, err := pluginsdk.CallbackMetadata(grant)
		if err != nil {
			release()
			return sdk.ParamsBase{}, nil, err
		}
		return metadata, release, nil
	})
	if err != nil || result.value.Published {
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
