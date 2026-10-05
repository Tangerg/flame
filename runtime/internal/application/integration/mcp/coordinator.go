// Package mcp coordinates durable server configuration, live connections, and
// the tool exposure consumed by run.
package mcp

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/application/taskgroup"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// StatusReader transfers a live status snapshot for configured MCP servers.
type StatusReader interface {
	Statuses() []mcpserver.ConnectionStatus
}

// ToolCatalog is the live tool set execution admits from. It borrows the
// optional server scope and transfers one snapshot: the advertised tools and
// the name conflicts that exclude some of them from Run manifests. A single
// source keeps the read model from disagreeing with what a Run would freeze.
// Implementations read admitted state without remote calls.
type ToolCatalog interface {
	MCPTools(server *mcpserver.ID) ([]mcpserver.AdvertisedTool, map[tool.Ref][]tool.Ref, error)
}

// ConnectionControl reconnects and authorizes configured servers.
// Implementations must sequence operations per server: a newer configure,
// remove, reconnect, or authorize supersedes an older in-flight operation, while
// operations for different servers may proceed concurrently. Each call blocks
// until its live status has settled and honors ctx cancellation; the application
// owns detachment, lifecycle, and asynchronous result publication.
type ConnectionControl interface {
	Reconnect(ctx context.Context, name mcpserver.ID) error
	Authorize(ctx context.Context, name mcpserver.ID) error
}

// Probe borrows an unsaved candidate. Configure obtains the current configuration
// from its source owner; the caller cannot advance it through a detached snapshot,
// and a Configure whose ctx was canceled before it settles records nothing, so
// canceling a dial before Detach keeps a detached source detached. Detach
// withdraws the source's tools before it returns and never blocks on I/O.
// Refuse settles a server whose connection cannot be admitted as failed in the
// given category and withdraws its session and tools; a refusal under a
// superseded operation's ctx records nothing.
type ConnectionLifecycle interface {
	Probe(ctx context.Context, server mcpserver.Server) error
	Configure(ctx context.Context, name mcpserver.ID) error
	Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error
	Detach(name mcpserver.ID) error
}

// Registry supplies durable definitions of every origin independently of
// executable admission.
type Registry interface {
	Catalog(ctx context.Context) ([]Source, error)
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

type SourceAvailability string

const (
	SourceAvailable          SourceAvailability = "available"
	SourceUnavailableRelease SourceAvailability = "unavailableRelease"
	SourceUnavailableBackend SourceAvailability = "unavailableBackend"
)

// Source separates durable registry membership from executable admission.
// An unavailable installation still supplies its desired configuration.
type Source struct {
	Server       mcpserver.Server
	Availability SourceAvailability
}

// Coordinator owns durable server configuration, live connections, and the
// atomically published tool exposure. Commands borrow input until they return;
// constructing a server acquires the data retained by persistence or a live dial.
type Coordinator struct {
	// mutationMu linearizes durable registry -> exposure/live reconciliation and
	// the short pre/post boundaries of asynchronous connection operations.
	// Network and interactive OAuth waits never hold it; ConnectionControl owns
	// per-server latest-operation-wins sequencing.
	registry              Registry
	store                 Store
	statusReader          StatusReader
	toolCatalog           ToolCatalog
	connectionControl     ConnectionControl
	connectionLifecycle   ConnectionLifecycle
	exposure              *ExposureState
	mutationMu            sync.Mutex
	connectionMu          sync.Mutex
	dials                 map[mcpserver.ID]*activeDial
	statusQueue           *statusQueue
	statusTombstones      map[mcpserver.ID]struct{}
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
	Store               Store
	StatusReader        StatusReader
	ToolCatalog         ToolCatalog
	ConnectionControl   ConnectionControl
	ConnectionLifecycle ConnectionLifecycle
	Exposure            *ExposureState
	// Invalidations publishes post-commit registry and live-connection changes.
	Invalidations invalidation.Publish
}

// New constructs the complete durable configuration and live-connection use cases.
func New(cfg Config) (*Coordinator, error) {
	if cfg.Registry == nil || cfg.Store == nil || cfg.StatusReader == nil || cfg.ToolCatalog == nil ||
		cfg.ConnectionControl == nil || cfg.ConnectionLifecycle == nil || cfg.Exposure == nil || cfg.Exposure.snapshot.Load() == nil {
		return nil, errors.New("mcp: registry, store, live connections, tool catalog, and exposure are required")
	}
	coordinator := &Coordinator{
		registry:              cfg.Registry,
		store:                 cfg.Store,
		statusReader:          cfg.StatusReader,
		toolCatalog:           cfg.ToolCatalog,
		connectionControl:     cfg.ConnectionControl,
		connectionLifecycle:   cfg.ConnectionLifecycle,
		exposure:              cfg.Exposure,
		dials:                 make(map[mcpserver.ID]*activeDial),
		statusTombstones:      make(map[mcpserver.ID]struct{}),
		authorizationAttempts: newAuthorizationAttemptStore(),
		invalidations:         cfg.Invalidations,
	}
	coordinator.statusQueue = newStatusQueue(func(name mcpserver.ID) {
		coordinator.invalidations.Notify(invalidation.ForMCP(name))
	})
	return coordinator, nil
}

// activeDial owns the current connection attempt's cancellation and temporary
// connecting phase. Both are guarded by connectionMu and end with the attempt.
type activeDial struct {
	cancel     context.CancelFunc
	connecting bool
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
