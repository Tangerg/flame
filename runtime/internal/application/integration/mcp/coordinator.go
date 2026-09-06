// Package mcp coordinates durable server configuration, live connections, and
// the tool policy consumed by run.
package mcp

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// StatusReader transfers a live status snapshot for configured MCP servers.
type StatusReader interface {
	Statuses() []mcpserver.ConnectionStatus
}

// ToolCatalog borrows the optional server scope and transfers the returned catalog.
type ToolCatalog interface {
	Tools(ctx context.Context, server *mcpserver.ServerName) ([]mcpserver.AdvertisedTool, error)
}

// ConnectionControl reconnects and authorizes configured servers.
// Implementations must sequence operations per server: a newer configure,
// remove, reconnect, or authorize supersedes an older in-flight operation, while
// operations for different servers may proceed concurrently. Each call blocks
// until its live status has settled and honors ctx cancellation; the application
// owns detachment, lifecycle, and asynchronous result publication.
type ConnectionControl interface {
	Reconnect(ctx context.Context, name mcpserver.ServerName) error
	Authorize(ctx context.Context, name mcpserver.ServerName) error
}

// ConnectionLifecycle borrows server descriptors during each synchronous call.
// Implementations acquire their own copy before retaining a live configuration.
type ConnectionLifecycle interface {
	Probe(ctx context.Context, server mcpserver.Server) error
	Configure(ctx context.Context, server mcpserver.Server) error
	Detach(name mcpserver.ServerName) error
}

// Registry transfers List and Get results and borrows Save input for the call.
type Registry interface {
	List(ctx context.Context) ([]mcpserver.Server, error)
	Get(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, bool, error)
	Save(ctx context.Context, server mcpserver.Server) error
	Remove(ctx context.Context, name mcpserver.ServerName) error
}

// Coordinator owns durable server configuration, live connections, and the
// atomically published tool policy. Commands borrow input until they return;
// constructing a server acquires the data retained by persistence or a live dial.
type Coordinator struct {
	// mutationMu linearizes durable registry -> policy/live reconciliation and
	// the short pre/post boundaries of asynchronous connection operations.
	// Network and interactive OAuth waits never hold it; ConnectionControl owns
	// per-server latest-operation-wins sequencing.
	registry              Registry
	statusReader          StatusReader
	toolCatalog           ToolCatalog
	connectionControl     ConnectionControl
	connectionLifecycle   ConnectionLifecycle
	policy                *ToolPolicyState
	mutationMu            sync.Mutex
	dialMu                sync.Mutex
	dials                 map[mcpserver.ServerName]*activeDial
	statusQueue           *statusQueue
	statusMu              sync.RWMutex
	statusOverrides       map[mcpserver.ServerName]ServerStatus
	authorizationAttempts *authorizationAttemptStore
	invalidations         invalidation.Publish

	// tasks is this component's context for post-commit reconcile: MCP registry
	// mutations outlive the request but are canceled and joined by the
	// BeginShutdown/AwaitShutdown lifecycle.
	tasks taskgroup.Group
}

// Config bundles the Coordinator's dependencies.
type Config struct {
	Registry            Registry
	StatusReader        StatusReader
	ToolCatalog         ToolCatalog
	ConnectionControl   ConnectionControl
	ConnectionLifecycle ConnectionLifecycle
	Policy              *ToolPolicyState
	// Invalidations publishes post-commit registry and live-connection changes.
	Invalidations invalidation.Publish
}

// New constructs the complete durable configuration and live-connection use cases.
func New(cfg Config) (*Coordinator, error) {
	if cfg.Registry == nil || cfg.StatusReader == nil || cfg.ToolCatalog == nil ||
		cfg.ConnectionControl == nil || cfg.ConnectionLifecycle == nil || cfg.Policy == nil {
		return nil, errors.New("mcp: registry, live connections, tool catalog, and policy are required")
	}
	coordinator := &Coordinator{
		registry:              cfg.Registry,
		statusReader:          cfg.StatusReader,
		toolCatalog:           cfg.ToolCatalog,
		connectionControl:     cfg.ConnectionControl,
		connectionLifecycle:   cfg.ConnectionLifecycle,
		policy:                cfg.Policy,
		dials:                 make(map[mcpserver.ServerName]*activeDial),
		statusOverrides:       make(map[mcpserver.ServerName]ServerStatus),
		authorizationAttempts: newAuthorizationAttemptStore(),
		invalidations:         cfg.Invalidations,
	}
	coordinator.statusQueue = newStatusQueue(coordinator.acceptStatus)
	return coordinator, nil
}

// activeDial is the cancellation handle for one server's current connection
// attempt.
type activeDial struct {
	cancel context.CancelFunc
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
