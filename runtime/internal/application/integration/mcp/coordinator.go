// Package mcp coordinates durable server configuration, live connections, and
// the tool exposure consumed by run.
package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// ToolCatalog is the live tool set execution admits from. It borrows the
// optional server scope and transfers one snapshot: the advertised tools and
// the name conflicts that exclude some of them from Run manifests. A single
// source keeps the read model from disagreeing with what a Run would freeze.
// Implementations read admitted state without remote calls.
type ToolCatalog interface {
	MCPTools(server *mcpserver.ID) ([]mcpserver.AdvertisedTool, map[tool.Ref][]tool.Ref, error)
}

// Attempt is an admitted connection operation. The connection owner publishes
// connecting before returning it. Run starts external I/O and joins its result;
// Close cancels it, including an operation that has not started.
type Attempt interface {
	Run() error
	Close()
}

// Connections owns all live transitions, tools and per-server supersession.
// Admission and withdrawal do no external I/O. Statuses is its read projection.
type Connections interface {
	Statuses() []mcpserver.ConnectionStatus
	BeginConnect(ctx context.Context, name mcpserver.ID) (Attempt, error)
	BeginAuthorize(ctx context.Context, name mcpserver.ID) (Attempt, error)
	Probe(ctx context.Context, server mcpserver.Server) error
	Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error
	Detach(name mcpserver.ID) error
}

// Registry supplies durable definitions of every origin independently of
// executable admission.
type Registry interface {
	Definitions(ctx context.Context) ([]mcpserver.Server, error)
	Definition(ctx context.Context, name mcpserver.ID) (mcpserver.Server, bool, error)
}

// Store persists what this coordinator owns directly: user-origin descriptors
// and the tool exposure relation over sources of every origin.
type Store interface {
	Save(ctx context.Context, server mcpserver.Server) error
	Remove(ctx context.Context, name mcpserver.ServerName) error
	ListExposure(ctx context.Context) ([]tool.Ref, error)
	SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool) error
}

// Coordinator owns user-server configuration and tool exposure, and coordinates
// the connection owner. Installations own installed-server definitions. Commands
// borrow input until they return; constructing a server acquires data retained
// by persistence or a live dial.
type Coordinator struct {
	// mutationMu linearizes durable registry -> exposure/live reconciliation and
	// the short pre/post boundaries of asynchronous connection operations.
	// Network and interactive OAuth waits never hold it; Connections owns
	// per-server latest-operation-wins sequencing.
	registry              Registry
	store                 Store
	toolCatalog           ToolCatalog
	connections           Connections
	exposure              *ExposureState
	mutationMu            sync.Mutex
	statusQueue           *statusQueue
	authorizationAttempts *authorizationAttemptStore
	invalidations         invalidation.Publish

	// tasks is this component's context for post-commit reconcile: MCP registry
	// mutations outlive the request but are canceled and joined by the
	// BeginShutdown/AwaitShutdown lifecycle.
	tasks taskgroup.Group
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Registry    Registry
	Store       Store
	ToolCatalog ToolCatalog
	Connections Connections
	Exposure    *ExposureState
	// Invalidations publishes post-commit registry and live-connection changes.
	Invalidations invalidation.Publish
}

// New constructs the complete durable configuration and live-connection use cases.
func New(cfg Config) (*Coordinator, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"registry", cfg.Registry}, {"store", cfg.Store},
		{"live connections", cfg.Connections}, {"tool catalog", cfg.ToolCatalog},
	} {
		if dependency.Missing(required.value) {
			return nil, fmt.Errorf("mcp: %s is required", required.name)
		}
	}
	if cfg.Exposure == nil || cfg.Exposure.snapshot.Load() == nil {
		return nil, errors.New("mcp: initialized exposure is required")
	}
	coordinator := &Coordinator{
		registry:              cfg.Registry,
		store:                 cfg.Store,
		toolCatalog:           cfg.ToolCatalog,
		connections:           cfg.Connections,
		exposure:              cfg.Exposure,
		authorizationAttempts: newAuthorizationAttemptStore(),
		invalidations:         cfg.Invalidations,
	}
	coordinator.statusQueue = newStatusQueue(func(name mcpserver.ID) {
		coordinator.invalidations.Notify(invalidation.ForMCP(name))
	})
	return coordinator, nil
}

// BeginShutdown cancels this component's post-commit reconcile work.
// It is idempotent.
func (c *Coordinator) BeginShutdown() {
	c.tasks.Cancel()
}

// AwaitShutdown joins post-commit reconcile work after [BeginShutdown].
func (c *Coordinator) AwaitShutdown(ctx context.Context) error {
	return c.tasks.Wait(ctx)
}
