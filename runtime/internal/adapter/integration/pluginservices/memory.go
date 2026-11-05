package pluginservices

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	framevalue "github.com/Tangerg/frame/value"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	memorycontract "github.com/Tangerg/flame/packages/go-sdk/plugin/memory"
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
	contract      *plugincontract.Capability
	installations plugins.Store
	pool          *mcp.Pool
	callbacks     *pluginmcp.Callbacks
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
	Callbacks     *pluginmcp.Callbacks
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
	contract, err := plugincontract.ParseCapability([]byte(memorycontract.CapabilityJSON))
	if err != nil {
		return nil, err
	}
	return &Memory{contract: contract, installations: config.Installations, pool: config.Connections, callbacks: config.Callbacks, utility: config.Utility, embedder: config.Embeddings, history: config.Conversation, commands: config.Commands}, nil
}

func (m *Memory) Items(ctx context.Context, scope memorycontract.ItemsInputScope, project string) ([]execution.MemoryPromptItem, error) {
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
	values, err := query[memorycontract.ItemsInput, memorycontract.ItemsV2Output](ctx, location, session, memorycontract.ItemsV2Method, memorycontract.ItemsInput{Scope: scope, Project: project}, sdk.ParamsBase{})
	if err != nil {
		return nil, err
	}
	items := make([]execution.MemoryPromptItem, len(values))
	for index, item := range values {
		if item == nil || item.Status != memorycontract.ItemsV2OutputDomainItemSnapshotStatusActive || item.Scope != memorycontract.ItemsV2OutputDomainItemSnapshotScope(scope) || item.Project != project {
			return nil, errors.New("memory: backend returned items outside the requested active target")
		}
		items[index], err = memoryPromptItem(string(item.ID), item.Content, item.Pinned, string(item.UpdatedAt))
		if err != nil {
			return nil, err
		}
	}
	return items, nil
}
func (m *Memory) Search(ctx context.Context, project, text string, topK int) (_ []execution.MemoryPromptItem, returned error) {
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
	if embedder != nil {
		defer func() { returned = errors.Join(returned, embedder.Close(context.WithoutCancel(ctx))) }()
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
	metadata, err := pluginmcp.CallbackMetadata(grant)
	if err != nil {
		return nil, err
	}
	limit, err := framevalue.ParseNumber([]byte(strconv.Itoa(topK)))
	if err != nil {
		return nil, err
	}
	values, err := query[memorycontract.SearchInput, memorycontract.SearchV2Output](ctx, location, session, memorycontract.SearchV2Method, memorycontract.SearchInput{Project: project, Query: text, TopK: limit, EmbeddingSpace: space}, metadata)
	if err != nil {
		return nil, err
	}
	if len(values) > max(topK, 0) {
		return nil, errors.New("memory: backend exceeded requested search bound")
	}
	items := make([]execution.MemoryPromptItem, len(values))
	for index, item := range values {
		if item == nil {
			return nil, errors.New("memory: backend returned an absent search item")
		}
		visibleTarget := item.Scope == memorycontract.SearchV2OutputDomainItemSnapshotScopeUser && item.Project == "" || item.Scope == memorycontract.SearchV2OutputDomainItemSnapshotScopeProject && item.Project == project
		if item.Status != memorycontract.SearchV2OutputDomainItemSnapshotStatusActive || !visibleTarget {
			return nil, errors.New("memory: backend returned search outside the active target")
		}
		items[index], err = memoryPromptItem(string(item.ID), item.Content, item.Pinned, string(item.UpdatedAt))
		if err != nil {
			return nil, err
		}
	}
	return items, nil
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
	count, err := framevalue.ParseNumber([]byte(strconv.Itoa(len(messages))))
	if err != nil {
		return err
	}
	request := memorycontract.MaintenanceInput{
		SessionID: input.SessionID, Project: input.WorkspaceCWD, Compacted: input.DurableContextCompacted,
		Conversation: &memorycontract.Conversation{MessageCount: count, Transcript: models.RenderTranscript(messages)},
	}
	_, err = invokeResolvedCommand[memorycontract.MaintenanceInput, memorycontract.MaintenanceResult](operation.Start(ctx, extension.Automatic, "plugins.automatic", uuid.NewString()), m.commands, memorycontract.MaintainV1Method, request, nil, func(context.Context) (plugins.ContributionBinding, error) { return binding, nil }, func(ctx context.Context, _ plugins.ContributionBinding, original *sdk.ClientSession) (sdk.ParamsBase, func() error, error) {
		model, releaseModel, err := m.utility(ctx, models.AuxiliaryBudget{MaxInputBytes: memorycontract.MaxModelInputBytes, MaxOutputTokens: memorycontract.MaxModelOutputTokens})
		if err != nil {
			return sdk.ParamsBase{}, nil, err
		}
		if releaseModel == nil {
			return sdk.ParamsBase{}, nil, errors.New("memory: captured utility model release is required")
		}
		grant, release, err := GrantMemoryModels(ctx, m.callbacks, original, model.Model(), nil)
		if err != nil {
			return sdk.ParamsBase{}, nil, errors.Join(err, releaseModel())
		}
		metadata, err := pluginmcp.CallbackMetadata(grant)
		if err != nil {
			release()
			return sdk.ParamsBase{}, nil, errors.Join(err, releaseModel())
		}
		return metadata, func() error {
			release()
			return releaseModel()
		}, nil
	})
	return err
}
func memoryPromptItem(id, content string, pinned bool, updated string) (execution.MemoryPromptItem, error) {
	instant, err := time.Parse(time.RFC3339Nano, updated)
	if err != nil || instant.IsZero() {
		return execution.MemoryPromptItem{}, errors.New("memory: published prompt item has no valid ordering time")
	}
	return execution.MemoryPromptItem{ID: id, Content: content, Pinned: pinned, UpdatedAt: instant}, nil
}
