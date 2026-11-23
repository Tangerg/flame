package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/samber/lo"
)

type toolCatalog interface {
	MCPTools(server *mcpserver.ID) ([]mcpserver.AdvertisedTool, map[tool.Ref][]tool.Ref, error)
}

type Attempt interface {
	Run() error
	Close()
}

type Connections interface {
	Statuses() []mcpserver.ConnectionStatus
	BeginConnect(ctx context.Context, name mcpserver.ID) (Attempt, error)
	BeginAuthorize(ctx context.Context, name mcpserver.ID) (Attempt, error)
	Probe(ctx context.Context, server mcpserver.Server) error
	Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error
	Detach(name mcpserver.ID) error
}

type registry interface {
	Definitions(ctx context.Context) ([]mcpserver.Server, error)
	Definition(ctx context.Context, name mcpserver.ID) (mcpserver.Server, bool, error)
}

type store interface {
	Save(ctx context.Context, server mcpserver.Server) error
	Remove(ctx context.Context, current mcpserver.Server) error
	ListExposure(ctx context.Context) ([]tool.Ref, error)
	SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool) error
}

type ExternalWorkflow interface {
	Invalidate()
	Affected(context.Context, []mcpserver.ID) (bool, error)
	List(context.Context) ([]mcpserver.Server, error)
	AdmitProbe(context.Context) (func() error, error)
	StartAuthorization(context.Context, mcpserver.Server, AuthorizationAttemptID) (AuthorizationAttempt, func(context.Context, AuthorizationAttemptStatus) error, error)
	ReadAuthorization(context.Context, AuthorizationAttemptID) (AuthorizationAttempt, error)
}

type Coordinator struct {
	// Network and interactive OAuth waits never hold mutationMu.
	registry                registry
	store                   store
	toolCatalog             toolCatalog
	connections             Connections
	exposure                *ExposureState
	mutationMu              sync.Mutex
	statusQueue             *statusQueue
	installedAuthorizations *authorizationAttemptStore
	external                ExternalWorkflow
	authorizationDeliveries pendingDeliveries
	invalidations           invalidation.Publish

	tasks taskgroup.Group
}

type Config struct {
	External      ExternalWorkflow
	Registry      registry
	Store         store
	ToolCatalog   toolCatalog
	Connections   Connections
	Exposure      *ExposureState
	Invalidations invalidation.Publish
}

func New(cfg Config) (*Coordinator, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"registry", cfg.Registry}, {"store", cfg.Store}, {"external workflow", cfg.External},
		{"live connections", cfg.Connections}, {"tool catalog", cfg.ToolCatalog},
		{"invalidations", cfg.Invalidations},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("mcp: %s is required", required.name)
		}
	}
	if cfg.Exposure == nil || cfg.Exposure.snapshot.Load() == nil {
		return nil, errors.New("mcp: initialized exposure is required")
	}
	coordinator := &Coordinator{
		external:                cfg.External,
		registry:                cfg.Registry,
		store:                   cfg.Store,
		toolCatalog:             cfg.ToolCatalog,
		connections:             cfg.Connections,
		exposure:                cfg.Exposure,
		installedAuthorizations: newAuthorizationAttemptStore(),
		invalidations:           cfg.Invalidations,
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
