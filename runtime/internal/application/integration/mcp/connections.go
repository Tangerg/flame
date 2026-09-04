package mcp

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// ReconnectServer re-dials a configured MCP server and hot-swaps the live tool
// set (mcp.servers.reconnect). Fire-and-forget: the name is validated
// synchronously (unknown → [ErrUnknownServer], disabled →
// [ErrServerDisabled]), then the dial runs on
// the component task group with connecting → settled status published for the
// status observers, so the initiating request does not abort it while shutdown
// still can.
func (c *Coordinator) ReconnectServer(ctx context.Context, name mcpserver.ServerName) error {
	return c.startConnection(ctx, name, func(ctx context.Context) error {
		return c.connectionControl.Reconnect(ctx, name)
	})
}

// startConnection validates the server exists, then runs dial on the
// component task group — detached from the caller's cancellation but keeping
// its trace values and canceled + joined by
// Close. It enters the application mutation order only for the pre/post registry
// checks and status publication; the dial itself runs outside that global
// critical section. The connection command's per-server generation makes a
// concurrent configure/remove supersede stale dial completion, while unrelated
// servers can connect in parallel. The task's context scopes both registry reads
// and dial.
// Returns [errConnectionUnavailable] when the coordinator lacks a required
// connection dependency, [ErrUnknownServer] or [ErrServerDisabled] when
// durable state refuses the command, or [errClosed] during shutdown.
func (c *Coordinator) startConnection(ctx context.Context, name mcpserver.ServerName, dial func(context.Context) error) error {
	if _, err := c.connectionTarget(ctx, name); err != nil {
		return err
	}
	return c.dispatchConnection(ctx, name, dial, true, nil, nil)
}

func (c *Coordinator) connectionTarget(ctx context.Context, name mcpserver.ServerName) (mcpserver.Server, error) {
	if c.registry == nil || c.statusReader == nil || c.connectionControl == nil {
		return mcpserver.Server{}, errConnectionUnavailable
	}
	if ctx == nil {
		return mcpserver.Server{}, errors.New("mcp: connection context is required")
	}
	registryCtx := context.WithoutCancel(ctx)
	srv, ok, err := c.registry.Get(registryCtx, name)
	if err != nil {
		return mcpserver.Server{}, fmt.Errorf("mcp: read MCP server %q: %w", name, err)
	}
	if !ok {
		return mcpserver.Server{}, ErrUnknownServer
	}
	if err := validateRegistryServer("get", name, srv); err != nil {
		return mcpserver.Server{}, err
	}
	if !srv.Enabled {
		return mcpserver.Server{}, ErrServerDisabled
	}
	return srv, nil
}

type connectionOutcome uint8

const (
	connectionSucceeded connectionOutcome = iota + 1
	connectionFailed
	connectionCanceled
)

// dispatchConnection runs a live (re)dial on the component task group, detached
// from the caller's cancellation. It enters the mutation order only for the
// pre/post registry checks and status publication; the dial itself runs OUTSIDE
// that global critical section, so a slow endpoint cannot freeze the control
// plane, and the registry re-read lets a concurrent configure/remove supersede a
// stale completion. A caller that already holds mutationMu may invoke this: the
// spawned task blocks on the lock until that caller releases it, then proceeds —
// which is exactly how the registry-write methods dispatch their live dial without
// holding the lock across the network handshake. When completed is non-nil, an
// admitted task calls it exactly once after it reaches succeeded, failed, or
// canceled; admission failure calls nothing. Returns errClosed only when the task
// group is shutting down.
func (c *Coordinator) dispatchConnection(
	ctx context.Context,
	name mcpserver.ServerName,
	connect func(context.Context) error,
	publishConnecting bool,
	start <-chan struct{},
	completed func(connectionOutcome),
) error {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return errClosed
	}
	dialCtx, operation := c.replaceDial(ownerCtx, name)
	command := connectionDispatch{
		coordinator:       c,
		name:              name,
		connect:           connect,
		publishConnecting: publishConnecting,
		start:             start,
		completed:         completed,
		operation:         operation,
		releaseOwner:      releaseOwner,
		outcome:           connectionCanceled,
	}
	if !c.tasks.StartLinked(dialCtx, command.run) {
		operation.cancel()
		c.clearDial(name, operation)
		releaseOwner()
		return errClosed
	}
	return nil
}

type connectionDispatch struct {
	coordinator       *Coordinator
	name              mcpserver.ServerName
	connect           func(context.Context) error
	publishConnecting bool
	start             <-chan struct{}
	completed         func(connectionOutcome)
	operation         *activeDial
	releaseOwner      func()
	outcome           connectionOutcome
}

func (command *connectionDispatch) run(ctx context.Context) {
	if command.completed != nil {
		defer func() { command.completed(command.outcome) }()
	}
	defer command.releaseOwner()
	defer command.coordinator.clearDial(command.name, command.operation)
	if !command.awaitStart(ctx) || ctx.Err() != nil {
		return
	}
	connecting, current, err := command.prepareConnecting(ctx)
	if err != nil {
		command.fail(ctx, err)
		return
	}
	if !current {
		return
	}
	command.coordinator.publishStatus(connecting)

	// Interactive OAuth may wait minutes for a human. The connection command
	// owns per-server generation and cancellation, so no application-wide
	// mutation lock is held while dialing. A configure/remove can supersede it
	// immediately; stale completion cannot swap itself back in.
	connectionErr := command.connect(ctx)
	if connectionErr != nil && ctx.Err() == nil {
		recordConnectionError(ctx, fmt.Errorf("mcp: connect MCP server %q: %w", command.name, connectionErr))
	}
	if ctx.Err() != nil {
		return
	}
	status := command.coordinator.liveStatus(command.name)
	settled, current, err := command.prepareSettled(ctx, status)
	if err != nil {
		command.fail(ctx, err)
		return
	}
	if !current {
		return
	}
	command.coordinator.publishStatus(settled)
	if connectionErr != nil || status.State != mcpserver.ConnectionConnected {
		command.outcome = connectionFailed
		return
	}
	command.outcome = connectionSucceeded
}

func (command *connectionDispatch) awaitStart(ctx context.Context) bool {
	if command.start == nil {
		return true
	}
	select {
	case <-command.start:
		return true
	case <-ctx.Done():
		return false
	}
}

func (command *connectionDispatch) prepareConnecting(ctx context.Context) (*statusEvent, bool, error) {
	coordinator := command.coordinator
	coordinator.mutationMu.Lock()
	defer coordinator.mutationMu.Unlock()
	srv, ok, err := coordinator.registry.Get(ctx, command.name)
	if err != nil {
		return nil, false, fmt.Errorf(
			"mcp: read MCP server %q before connection: %w",
			command.name,
			err,
		)
	}
	if ok {
		if err := validateRegistryServer("get before connection", command.name, srv); err != nil {
			return nil, false, err
		}
	}
	if !ok || !srv.Enabled || !coordinator.currentDial(command.name, command.operation) {
		return nil, false, nil
	}
	if !command.publishConnecting {
		return nil, true, nil
	}
	return coordinator.prepareStatus(ServerStatus{
		Name:  command.name,
		Known: true,
		State: mcpserver.ConnectionConnecting,
	}), true, nil
}

func (command *connectionDispatch) prepareSettled(
	ctx context.Context,
	status ServerStatus,
) (*statusEvent, bool, error) {
	coordinator := command.coordinator
	coordinator.mutationMu.Lock()
	defer coordinator.mutationMu.Unlock()
	srv, ok, err := coordinator.registry.Get(ctx, command.name)
	if err != nil {
		return nil, false, fmt.Errorf(
			"mcp: read MCP server %q after connection: %w",
			command.name,
			err,
		)
	}
	if ok {
		if err := validateRegistryServer("get after connection", command.name, srv); err != nil {
			return nil, false, err
		}
	}
	if !ok || !srv.Enabled || !coordinator.currentDial(command.name, command.operation) {
		return nil, false, nil
	}
	return coordinator.prepareStatus(status), true, nil
}

func (command *connectionDispatch) fail(ctx context.Context, err error) {
	recordConnectionError(ctx, err)
	command.outcome = connectionFailed
}

// replaceDial gives each server exactly one current connection operation.
// A registry mutation, reconnect, or authorization attempt supersedes the previous dial by
// canceling its context; connection commands must honor ctx while dialing and
// reject a stale completion through their per-server generation check.
func (c *Coordinator) replaceDial(ctx context.Context, name mcpserver.ServerName) (context.Context, *activeDial) {
	dialCtx, cancel := context.WithCancel(ctx)
	dial := &activeDial{cancel: cancel}
	c.dialMu.Lock()
	if previous := c.dials[name]; previous != nil {
		previous.cancel()
	}
	c.dials[name] = dial
	c.dialMu.Unlock()
	return dialCtx, dial
}

func (c *Coordinator) cancelDial(name mcpserver.ServerName) {
	c.dialMu.Lock()
	if dial := c.dials[name]; dial != nil {
		dial.cancel()
		delete(c.dials, name)
	}
	c.dialMu.Unlock()
}

func (c *Coordinator) clearDial(name mcpserver.ServerName, dial *activeDial) {
	c.dialMu.Lock()
	if c.dials[name] == dial {
		delete(c.dials, name)
	}
	c.dialMu.Unlock()
}

func (c *Coordinator) currentDial(name mcpserver.ServerName, dial *activeDial) bool {
	c.dialMu.Lock()
	defer c.dialMu.Unlock()
	return c.dials[name] == dial
}

func recordConnectionError(ctx context.Context, err error) {
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
	}
}

type statusEvent struct {
	status ServerStatus
	next   *statusEvent
	ready  bool
}

type statusQueue struct {
	mu       sync.Mutex
	head     *statusEvent
	tail     *statusEvent
	draining bool
	sink     func(ServerStatus)
}

func newStatusQueue(sink func(ServerStatus)) *statusQueue {
	return &statusQueue{sink: sink}
}

// prepareStatus is called while mutationMu is held. Queue registration captures
// that exact mutation order before callback publication becomes lock-free.
func (c *Coordinator) prepareStatus(status ServerStatus) *statusEvent {
	return c.statusQueue.prepare(status)
}

func (c *Coordinator) publishStatus(event *statusEvent) {
	c.statusQueue.publish(event)
}

func (s *statusQueue) prepare(status ServerStatus) *statusEvent {
	event := &statusEvent{status: status}
	if s == nil || s.sink == nil {
		return event
	}
	s.mu.Lock()
	if s.tail == nil {
		s.head = event
	} else {
		s.tail.next = event
	}
	s.tail = event
	s.mu.Unlock()
	return event
}

func (s *statusQueue) publish(event *statusEvent) {
	if s == nil || s.sink == nil || event == nil {
		return
	}
	s.mu.Lock()
	event.ready = true
	if s.draining {
		s.mu.Unlock()
		return
	}
	s.draining = true
	s.mu.Unlock()

	for {
		s.mu.Lock()
		event := s.head
		if event == nil || !event.ready {
			s.draining = false
			s.mu.Unlock()
			return
		}
		s.head = event.next
		event.next = nil
		if s.head == nil {
			s.tail = nil
		}
		s.mu.Unlock()
		s.sink(event.status)
	}
}
