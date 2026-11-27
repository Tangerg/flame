package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/samber/lo"
)

type toolCatalog interface {
	MCPTools(ctx context.Context, server *mcpserver.ID) ([]mcpserver.AdvertisedTool, map[tool.Ref][]tool.Ref, error)
}

type Attempt interface {
	Run() error
	Close()
}

type Connections interface {
	Statuses() []mcpserver.ConnectionStatus
	// A matching attempt keeps its ownership, so there is no new attempt to run.
	EnsureConnection(ctx context.Context, target mcpserver.Server) (Attempt, error)
	BeginReconnect(ctx context.Context, target mcpserver.Server) (Attempt, error)
	BeginAuthorize(ctx context.Context, name mcpserver.ID, recipient framevalue.Digest) (Attempt, error)
	Probe(ctx context.Context, server mcpserver.Server) error
	Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error
	Detach(name mcpserver.ID) error
}

type registry interface {
	Definitions(ctx context.Context) ([]mcpserver.Server, error)
	Definition(ctx context.Context, name mcpserver.ID) (mcpserver.Server, bool, error)
}

// The Connector supplies the decision before Runtime publishes its live effects
// and completes the local launch authorization and acknowledgement.
type DirectorySettlement struct {
	Source   mcpserver.Source
	Name     mcpserver.ServerName
	Refusal  error
	Complete func(context.Context) error
}

func (s DirectorySettlement) ServerID() mcpserver.ID { return s.Source.ID(s.Name) }

type directoryStore interface {
	Save(context.Context, mcpserver.Server, operation.ReplayClaim) (*DirectorySettlement, error)
	Remove(context.Context, mcpserver.Server, operation.ReplayClaim) (*DirectorySettlement, error)
	RecoverDirectoryChange(context.Context, operation.ReplayClaim) (*DirectorySettlement, error)
}

type exposureStore interface {
	ListExposure(ctx context.Context) ([]tool.Ref, error)
	SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool) error
}

type ExternalWorkflow interface {
	Invalidate()
	Affected(context.Context, []mcpserver.ID) (bool, error)
	List(context.Context) ([]mcpserver.Server, error)
	AdmitProbe(context.Context) (func() error, error)
	StartAuthorization(context.Context, mcpserver.Server, AuthorizationAttemptID, operation.ReplayClaim) (*AuthorizationStartSettlement, error)
	RecoverAuthorizationStart(context.Context, operation.ReplayClaim) (*AuthorizationStartSettlement, error)
	RestoreCommands(context.Context) error
	RunAuthorizationDelivery(context.Context)
	ReadAuthorization(context.Context, AuthorizationAttemptID) (AuthorizationAttempt, error)
}

type Coordinator struct {
	registry      registry
	directory     directoryStore
	exposureStore exposureStore
	toolCatalog   toolCatalog
	connections   Connections
	mutationMu    sync.Mutex
	statusQueue   *statusQueue
	external      ExternalWorkflow
	invalidations invalidation.Publish

	tasks taskgroup.Group
}

type Config struct {
	External      ExternalWorkflow
	Registry      registry
	Directory     directoryStore
	ExposureStore exposureStore
	ToolCatalog   toolCatalog
	Connections   Connections
	Invalidations invalidation.Publish
}

func New(cfg Config) (*Coordinator, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"registry", cfg.Registry}, {"directory", cfg.Directory}, {"exposure store", cfg.ExposureStore},
		{"external workflow", cfg.External},
		{"live connections", cfg.Connections}, {"tool catalog", cfg.ToolCatalog},
		{"invalidations", cfg.Invalidations},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("mcp: %s is required", required.name)
		}
	}
	coordinator := &Coordinator{
		external:      cfg.External,
		registry:      cfg.Registry,
		directory:     cfg.Directory,
		exposureStore: cfg.ExposureStore,
		toolCatalog:   cfg.ToolCatalog,
		connections:   cfg.Connections,
		invalidations: cfg.Invalidations,
	}
	coordinator.statusQueue = newStatusQueue(func(name mcpserver.ID) {
		coordinator.invalidations.Notify(invalidation.ForMCP(name))
	})
	return coordinator, nil
}

func (c *Coordinator) BeginShutdown() {
	c.tasks.Cancel()
}

func (c *Coordinator) AwaitShutdown(ctx context.Context) error {
	return c.tasks.Wait(ctx)
}

type mutationScope struct {
	coordinator *Coordinator
	requestCtx  context.Context
	ownerCtx    context.Context
	finish      func()
	locked      bool
}

func (c *Coordinator) beginMutation(ctx context.Context) (*mutationScope, error) {
	if ctx == nil {
		return nil, errors.New("mcp: mutation context is required")
	}
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return nil, errClosed
	}
	requestCtx, releaseRequest, ok := c.tasks.AttachLinked(ctx)
	if !ok {
		releaseOwner()
		return nil, errClosed
	}
	c.mutationMu.Lock()
	if err := requestCtx.Err(); err != nil {
		c.mutationMu.Unlock()
		releaseRequest()
		releaseOwner()
		return nil, err
	}
	return &mutationScope{
		coordinator: c,
		requestCtx:  requestCtx,
		ownerCtx:    ownerCtx,
		finish: func() {
			releaseRequest()
			releaseOwner()
		},
		locked: true,
	}, nil
}

func (m *mutationScope) unlock() {
	if m != nil && m.locked {
		m.locked = false
		m.coordinator.mutationMu.Unlock()
	}
}

func (m *mutationScope) close() {
	if m == nil || m.finish == nil {
		return
	}
	m.unlock()
	m.finish()
	m.finish = nil
}
