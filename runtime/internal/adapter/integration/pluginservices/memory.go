package pluginservices

import (
	"context"
	"errors"
	"fmt"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/pluginservices/memorycontract"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	modeltext "github.com/Tangerg/flame/runtime/model/text"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
)

type Memory struct {
	contract      *capability.Contract
	installations plugins.Store
	pool          *mcpconnection.Pool
	callbacks     *pluginsdk.Callbacks
	utility       models.AuxiliaryResolver
	embedder      func(context.Context) (models.TextEmbedder, error)
	history       ConversationReader
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
	Embeddings    func(context.Context) (models.TextEmbedder, error)
	Conversation  ConversationReader
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
	contract, err := capability.Parse([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return nil, err
	}
	return &Memory{contract: contract, installations: config.Installations, pool: config.Connections, callbacks: config.Callbacks, utility: config.Utility, embedder: config.Embeddings, history: config.Conversation, commands: config.Commands}, nil
}

func (m *Memory) Items(ctx context.Context, scope, project string) ([]execution.MemoryPromptItem, error) {
	ctx = operation.Start(ctx, capability.Automatic, "memory.context", uuid.NewString())
	location, session, err := contributionSession(ctx, m.installations, m.commands.selections, m.pool, m.contract)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	values, err := query[memorycontract.ItemsV1Input, memorycontract.ItemsV1Output](ctx, location, session, memorycontract.ItemsV1Method, memorycontract.ItemsV1Input{Scope: scope, Project: project}, sdk.ParamsBase{})
	if err != nil {
		return nil, err
	}
	for _, item := range values {
		if item.Status != "active" || item.Scope != scope || item.Project != project {
			return nil, errors.New("memory: backend returned items outside the requested active target")
		}
	}
	return memoryPromptItems(memorycontract.ItemsV1Output(values))
}
func (m *Memory) Search(ctx context.Context, project, text string, topK int) ([]execution.MemoryPromptItem, error) {
	location, session, err := contributionSession(ctx, m.installations, m.commands.selections, m.pool, m.contract)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ctx, err = queryContext(ctx, location)
	if err != nil {
		return nil, err
	}
	embedder, err := m.embedder(ctx)
	if err != nil {
		return nil, err
	}
	grant, release, err := GrantMemoryModels(ctx, m.callbacks, session, nil, embedder)
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
	values, err := query[memorycontract.SearchV1Input, memorycontract.SearchV1Output](ctx, location, session, memorycontract.SearchV1Method, memorycontract.SearchV1Input{Project: project, Query: text, TopK: int64(topK), EmbeddingSpace: space}, metadata)
	if err != nil {
		return nil, err
	}
	if len(values) > max(topK, 0) {
		return nil, errors.New("memory: backend exceeded requested search bound")
	}
	for _, item := range values {
		visibleTarget := item.Scope == "user" && item.Project == "" || item.Scope == "project" && item.Project == project
		if item.Status != "active" || !visibleTarget {
			return nil, errors.New("memory: backend returned search outside the active target")
		}
	}
	return memoryPromptItems(memorycontract.ItemsV1Output(values))
}
func (m *Memory) MaintainMemory(ctx context.Context, input execution.RunMaintenanceInput) error {
	binding, _, err := contributionSession(ctx, m.installations, m.commands.selections, m.pool, m.contract)
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
	request := memorycontract.MaintainV1Input{SessionID: input.SessionID, Project: input.WorkspaceCWD, Compacted: input.DurableContextCompacted}
	request.Conversation.MessageCount = int64(len(messages))
	request.Conversation.Transcript = modeltext.RenderTranscript(messages)
	_, err = invokeResolvedCommand[memorycontract.MaintainV1Input, memorycontract.MaintainV1Output](operation.Start(ctx, capability.Automatic, "plugins.automatic", uuid.NewString()), m.commands, memorycontract.MaintainV1Method, request, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, func(ctx context.Context, _ plugins.ContributionBinding, original *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
		model, releaseModel, err := m.utility(ctx, models.AuxiliaryBudget{MaxInputBytes: memorycontract.MaxModelInputBytes, MaxOutputTokens: memorycontract.MaxModelOutputTokens})
		if err != nil {
			return sdk.ParamsBase{}, nil, err
		}
		if releaseModel == nil {
			return sdk.ParamsBase{}, nil, errors.New("memory: captured utility model release is required")
		}
		grant, release, err := GrantMemoryModels(ctx, m.callbacks, original, model.Model(), nil)
		if err != nil {
			releaseModel()
			return sdk.ParamsBase{}, nil, err
		}
		metadata, err := pluginsdk.CallbackMetadata(grant)
		if err != nil {
			release()
			releaseModel()
			return sdk.ParamsBase{}, nil, err
		}
		return metadata, func() {
			release()
			releaseModel()
		}, nil
	})
	return err
}
func memoryPromptItems(values memorycontract.ItemsV1Output) ([]execution.MemoryPromptItem, error) {
	items := make([]execution.MemoryPromptItem, len(values))
	for i, value := range values {
		if value.UpdatedAt.IsZero() {
			return nil, errors.New("memory: published prompt item has no ordering time")
		}
		items[i] = execution.MemoryPromptItem{ID: value.ID, Content: value.Content, Pinned: value.Pinned, UpdatedAt: value.UpdatedAt}
	}
	return items, nil
}
