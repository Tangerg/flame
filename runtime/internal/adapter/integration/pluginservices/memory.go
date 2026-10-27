package pluginservices

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	models "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	execution "github.com/Tangerg/flame/runtime/internal/adapter/run/execution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
	"github.com/google/uuid"
	"github.com/samber/lo"
)

type Memory struct {
	contract      *portable.Capability
	installations plugins.Store
	pool          *mcp.Pool
	callbacks     *qimumcp.Callbacks
	utility       models.AuxiliaryResolver
	embedder      func(context.Context) (models.TextEmbedder, error)
	history       conversationReader
	commands      *CommandCalls
}

type conversationReader interface {
	Read(context.Context, string) ([]chat.Message, error)
}
type MemoryConfig struct {
	Installations plugins.Store
	Connections   *mcp.Pool
	Callbacks     *qimumcp.Callbacks
	Utility       models.AuxiliaryResolver
	Embeddings    func(context.Context) (models.TextEmbedder, error)
	Conversation  conversationReader
	Commands      *CommandCalls
}

func NewMemory(config MemoryConfig) (*Memory, error) {
	for _, required := range []struct {
		name  string
		value any
	}{{"installation store", config.Installations}, {"connections", config.Connections}, {"callbacks", config.Callbacks}, {"utility resolver", config.Utility}, {"embedding resolver", config.Embeddings}, {"conversation reader", config.Conversation}} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("memory: %s is required", required.name)
		}
	}
	if config.Commands == nil {
		return nil, errors.New("memory: command routing is required")
	}
	contract, err := portable.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return nil, err
	}
	return &Memory{contract: contract, installations: config.Installations, pool: config.Connections, callbacks: config.Callbacks, utility: config.Utility, embedder: config.Embeddings, history: config.Conversation, commands: config.Commands}, nil
}

func (m *Memory) Items(ctx context.Context, scope, project string) ([]execution.MemoryPromptItem, error) {
	ctx = operation.Start(ctx, extension.Automatic, "memory.context", uuid.NewString())
	location, borrowed, err := contributionSession(ctx, m.commands.framework, m.installations, m.commands.selections, m.pool, m.contract)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	session := borrowed.Session()
	values, err := query[memorycontract.ItemsV2Input, memorycontract.ItemsV2Output](ctx, location, session, memorycontract.ItemsV2Method, memorycontract.ItemsV2Input{Scope: scope, Project: project}, sdk.ParamsBase{})
	if err != nil {
		return nil, err
	}
	for _, item := range values {
		if item.Status != "active" || item.Scope != scope || item.Project != project {
			return nil, errors.New("memory: backend returned items outside the requested active target")
		}
	}
	return memoryPromptItems(memorycontract.ItemsV2Output(values))
}
func (m *Memory) Search(ctx context.Context, project, text string, topK int) ([]execution.MemoryPromptItem, error) {
	location, borrowed, err := contributionSession(ctx, m.commands.framework, m.installations, m.commands.selections, m.pool, m.contract)
	if errors.Is(err, errInactive) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	session := borrowed.Session()
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
	metadata, err := qimumcp.CallbackMetadata(grant)
	if err != nil {
		return nil, err
	}
	values, err := query[memorycontract.SearchV2Input, memorycontract.SearchV2Output](ctx, location, session, memorycontract.SearchV2Method, memorycontract.SearchV2Input{Project: project, Query: text, TopK: int64(topK), EmbeddingSpace: space}, metadata)
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
	return memoryPromptItems(memorycontract.ItemsV2Output(values))
}
func (m *Memory) MaintainMemory(ctx context.Context, input execution.RunMaintenanceInput) error {
	binding, borrowed, err := contributionSession(ctx, m.commands.framework, m.installations, m.commands.selections, m.pool, m.contract)
	if errors.Is(err, errInactive) {
		return nil
	}
	if err != nil {
		return err
	}
	defer borrowed.Close()
	ctx = borrowed.Context()
	messages, err := m.history.Read(ctx, input.SessionID)
	if err != nil {
		return fmt.Errorf("memory: capture committed conversation: %w", err)
	}
	request := memorycontract.MaintainV1Input{SessionID: input.SessionID, Project: input.WorkspaceCWD, Compacted: input.DurableContextCompacted}
	request.Conversation.MessageCount = int64(len(messages))
	request.Conversation.Transcript = models.RenderTranscript(messages)
	_, err = invokeResolvedCommand[memorycontract.MaintainV1Input, memorycontract.MaintainV1Output](operation.Start(ctx, extension.Automatic, "plugins.automatic", uuid.NewString()), m.commands, memorycontract.MaintainV1Method, request, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, func(ctx context.Context, _ plugins.ContributionBinding, original *sdk.ClientSession) (sdk.ParamsBase, func(), error) {
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
		metadata, err := qimumcp.CallbackMetadata(grant)
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
func memoryPromptItems(values memorycontract.ItemsV2Output) ([]execution.MemoryPromptItem, error) {
	items := make([]execution.MemoryPromptItem, len(values))
	for i, value := range values {
		if value.UpdatedAt.IsZero() {
			return nil, errors.New("memory: published prompt item has no ordering time")
		}
		items[i] = execution.MemoryPromptItem{ID: string(value.ID), Content: value.Content, Pinned: value.Pinned, UpdatedAt: value.UpdatedAt}
	}
	return items, nil
}
