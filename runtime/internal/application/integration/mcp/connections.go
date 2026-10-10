package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func (c *Coordinator) ReconnectServer(ctx context.Context, name mcpserver.ID) error {
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if _, err := c.connectionTarget(write.requestCtx, name); err != nil {
		return err
	}
	start := make(chan struct{})
	defer close(start)
	var event *statusEvent
	if err := c.dispatchConnection(write.ownerCtx, name, c.connections.BeginConnect, start, func() { c.statusQueue.publish(event) }, nil); err != nil {
		return err
	}
	event = c.statusQueue.prepare(name)
	write.unlock()
	return nil
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

// dispatchConnection admits under mutationMu, then waits for the caller to
// publish admission before external I/O. The connection owner alone fences stale
// completions; notifications carry identity and cannot advance connection state.
func (c *Coordinator) dispatchConnection(
	ctx context.Context,
	name mcpserver.ID,
	begin func(context.Context, mcpserver.ID) (Attempt, error),
	start <-chan struct{},
	notifyAdmission func(),
	completed func(connectionOutcome),
) error {
	ownerCtx, releaseOwner, ok := c.tasks.Attach(ctx)
	if !ok {
		return errClosed
	}
	attempt, err := begin(ownerCtx, name)
	if err != nil {
		releaseOwner()
		return err
	}
	if !c.tasks.StartLinked(ownerCtx, func(ctx context.Context) {
		defer releaseOwner()
		defer attempt.Close()
		outcome := connectionCanceled
		if completed != nil {
			defer func() { completed(outcome) }()
		}
		select {
		case <-ctx.Done():
			return
		case <-start:
		}
		if notifyAdmission != nil {
			notifyAdmission()
		}
		err := attempt.Run()
		switch {
		case err == nil:
			outcome = connectionSucceeded
		case errors.Is(err, context.Canceled):
			outcome = connectionCanceled
		default:
			outcome = connectionFailed
			slog.ErrorContext(ctx, "mcp: connection operation failed", "server.name", name.String(), "error", err)
		}
		c.mutationMu.Lock()
		event := c.statusQueue.prepare(name)
		c.mutationMu.Unlock()
		c.statusQueue.publish(event)
	}) {
		attempt.Close()
		releaseOwner()
		return errClosed
	}
	return nil
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
