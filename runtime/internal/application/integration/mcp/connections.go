package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// ReconnectServer re-dials a configured MCP server and hot-swaps the live tool
// set (mcp.servers.reconnect). Fire-and-forget: the name is validated
// synchronously (unknown → [ErrUnknownServer], disabled →
// [ErrServerDisabled]), then the dial runs on
// the component task group with connecting → settled status published for the
// status observers, so the initiating request does not abort it while shutdown
// still can.
func (c *Coordinator) ReconnectServer(ctx context.Context, name mcpserver.ID) error {
	return c.startConnection(ctx, name, func(ctx context.Context) error {
		return c.connectionControl.Reconnect(ctx, name)
	})
}

// startConnection validates the server exists, then runs dial on the
// component task group — detached from the caller's cancellation but keeping
// its request values and canceled + joined by BeginShutdown/AwaitShutdown.
// It enters the application mutation order only for the pre/post registry checks
// and status publication; the dial itself runs outside that global
// critical section. The connection command's per-server generation makes a
// concurrent configure/remove supersede stale dial completion, while unrelated
// servers can connect in parallel. The task's context scopes both registry reads
// and dial.
// Returns [ErrUnknownServer] or [ErrServerDisabled] when
// durable state refuses the command, or [errClosed] during shutdown.
func (c *Coordinator) startConnection(ctx context.Context, name mcpserver.ID, dial func(context.Context) error) error {
	if _, err := c.connectionTarget(ctx, name); err != nil {
		return err
	}
	_, err := c.dispatchConnection(ctx, name, dial, true, nil, nil)
	return err
}

func (c *Coordinator) connectionTarget(ctx context.Context, name mcpserver.ID) (mcpserver.Server, error) {
	if ctx == nil {
		return mcpserver.Server{}, errors.New("mcp: connection context is required")
	}
	srv, ok, err := c.registry.Definition(ctx, name)
	if err != nil {
		return mcpserver.Server{}, fmt.Errorf("mcp: read MCP server %q: %w", name, err)
	}
	if !ok {
		return mcpserver.Server{}, ErrUnknownServer
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
	name mcpserver.ID,
	connect func(context.Context) error,
	publishConnecting bool,
	start <-chan struct{},
	completed func(connectionOutcome),
) (*activeDial, error) {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return nil, errClosed
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
		return nil, errClosed
	}
	return operation, nil
}

type connectionDispatch struct {
	coordinator       *Coordinator
	name              mcpserver.ID
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
	command.coordinator.statusQueue.publish(connecting)

	// Interactive OAuth may wait minutes for a human. The connection command
	// owns per-server generation and cancellation, so no application-wide
	// mutation lock is held while dialing. A configure/remove can supersede it
	// immediately; stale completion cannot swap itself back in.
	connectionErr := command.connect(ctx)
	if connectionErr != nil && ctx.Err() == nil {
		slog.ErrorContext(ctx, "mcp: connection failed",
			"server.name", command.name.String(), "error", connectionErr,
		)
	}
	if ctx.Err() != nil {
		return
	}
	status, err := command.coordinator.liveStatus(command.name)
	if err != nil {
		command.fail(ctx, err)
		return
	}
	settled, current, err := command.prepareSettled(ctx, status)
	if err != nil {
		command.fail(ctx, err)
		return
	}
	if !current {
		return
	}
	command.coordinator.statusQueue.publish(settled)
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
	srv, ok, err := coordinator.registry.Definition(ctx, command.name)
	if err != nil {
		return nil, false, fmt.Errorf(
			"mcp: read MCP server %q before connection: %w",
			command.name,
			err,
		)
	}
	if !ok || !srv.Enabled || !coordinator.currentDial(command.name, command.operation) {
		return nil, false, nil
	}
	if !command.publishConnecting {
		return nil, true, nil
	}
	return coordinator.prepareStatus(ServerStatus{
		Server: command.name,
		Known:  true,
		State:  mcpserver.ConnectionConnecting,
	}, command.operation), true, nil
}

func (command *connectionDispatch) prepareSettled(
	ctx context.Context,
	status ServerStatus,
) (*statusEvent, bool, error) {
	coordinator := command.coordinator
	coordinator.mutationMu.Lock()
	defer coordinator.mutationMu.Unlock()
	srv, ok, err := coordinator.registry.Definition(ctx, command.name)
	if err != nil {
		return nil, false, fmt.Errorf(
			"mcp: read MCP server %q after connection: %w",
			command.name,
			err,
		)
	}
	if !ok || !srv.Enabled || !coordinator.currentDial(command.name, command.operation) {
		return nil, false, nil
	}
	return coordinator.prepareStatus(status, command.operation), true, nil
}

// fail records a dispatch that could not establish whether its source still
// admits a connection. The cause stays in the log because it can carry paths or
// credentials; the refusal reaches status readers as a configuration failure
// and withdraws the session, because an unverifiable source must not keep
// serving tools.
func (command *connectionDispatch) fail(ctx context.Context, err error) {
	// Registry reads share the dial's cancellation and must not turn a
	// superseded authorization attempt into a failed one.
	if ctx.Err() != nil {
		return
	}
	slog.ErrorContext(ctx, "mcp: connection failed",
		"server.name", command.name.String(), "error", err,
	)
	command.outcome = connectionFailed
	coordinator := command.coordinator
	coordinator.mutationMu.Lock()
	var event *statusEvent
	if coordinator.currentDial(command.name, command.operation) {
		if refuseErr := coordinator.connectionLifecycle.Refuse(ctx, command.name, mcpserver.FailureConfiguration); refuseErr != nil {
			slog.ErrorContext(ctx, "mcp: connection refusal was not recorded",
				"server.name", command.name.String(), "error", refuseErr,
			)
		}
		status, statusErr := coordinator.liveStatus(command.name)
		if statusErr == nil {
			event = coordinator.prepareStatus(status, command.operation)
		}
	}
	coordinator.mutationMu.Unlock()
	coordinator.statusQueue.publish(event)
}

// replaceDial gives each server exactly one current connection operation.
// A registry mutation, reconnect, or authorization attempt supersedes the previous dial by
// canceling its context; connection commands must honor ctx while dialing and
// reject a stale completion through their per-server generation check.
func (c *Coordinator) replaceDial(ctx context.Context, name mcpserver.ID) (context.Context, *activeDial) {
	dialCtx, cancel := context.WithCancel(ctx)
	dial := &activeDial{cancel: cancel}
	c.connectionMu.Lock()
	if previous := c.dials[name]; previous != nil {
		previous.cancel()
	}
	c.dials[name] = dial
	c.connectionMu.Unlock()
	return dialCtx, dial
}

func (c *Coordinator) cancelDial(name mcpserver.ID) {
	c.connectionMu.Lock()
	if dial := c.dials[name]; dial != nil {
		dial.cancel()
		delete(c.dials, name)
	}
	c.connectionMu.Unlock()
}

func (c *Coordinator) clearDial(name mcpserver.ID, dial *activeDial) {
	c.connectionMu.Lock()
	retiredConnecting := false
	if c.dials[name] == dial {
		retiredConnecting = dial.connecting
		delete(c.dials, name)
	}
	c.connectionMu.Unlock()
	if retiredConnecting {
		c.invalidations.Notify(invalidation.ForMCP(name))
	}
}

func (c *Coordinator) currentDial(name mcpserver.ID, dial *activeDial) bool {
	c.connectionMu.Lock()
	defer c.connectionMu.Unlock()
	return c.dials[name] == dial
}

type statusEvent struct {
	name  mcpserver.ID
	next  *statusEvent
	ready bool
}

type statusQueue struct {
	mu       sync.Mutex
	head     *statusEvent
	tail     *statusEvent
	draining bool
	sink     func(mcpserver.ID)
}

func newStatusQueue(sink func(mcpserver.ID)) *statusQueue {
	return &statusQueue{sink: sink}
}

func (s *statusQueue) prepare(name mcpserver.ID) *statusEvent {
	event := &statusEvent{name: name}
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
		s.sink(event.name)
	}
}
