package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func (c *connections) statuses() []mcpserver.ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]mcpserver.ConnectionStatus, 0, len(c.servers))
	for _, configuredServer := range c.servers {
		out = append(out, mcpserver.ConnectionStatus{
			Server:    configuredServer.name(),
			State:     configuredServer.state,
			Failure:   configuredServer.failure,
			ToolCount: len(configuredServer.tools),
		})
	}
	return out
}

func (c *connections) detach(name mcpserver.ID) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errConnectionsClosed
	}
	if index := slices.IndexFunc(c.servers, func(s *server) bool { return s.name() == name }); index >= 0 {
		c.removeLocked(c.servers[index])
	}
	c.mu.Unlock()
	return nil
}

func (c *connections) removeLocked(target *server) {
	if target.attempt != nil {
		target.attempt.cancel()
		target.attempt = nil
	}
	c.retireSessionLocked(target.session)
	index := slices.Index(c.servers, target)
	c.servers = slices.Delete(c.servers, index, index+1)
	c.publishToolsLocked(target.id)
}

func (c *connections) retireSessionLocked(session *sdk.ClientSession) *sessionCloseAttempt {
	attempt := c.beginSessionCloseLocked(session)
	if attempt != nil {
		if c.retirements == nil {
			c.retirements = make(map[*sessionCloseAttempt]struct{})
		}
		c.retirements[attempt] = struct{}{}
	}
	return attempt
}

// Network I/O stays outside this boundary so caller cancellation or another
// server's failure cannot become a false catalog.
func (c *connections) publishToolsLocked(changed ...mcpserver.ID) {
	if c.onTools == nil {
		return
	}
	var catalog []Executable
	var pending []mcpserver.ID
	for _, configuredServer := range c.servers {
		if configuredServer.state == mcpserver.ConnectionConnecting {
			pending = append(pending, configuredServer.id)
		}
		if configuredServer.session != nil {
			catalog = append(catalog, configuredServer.tools...)
		}
	}
	c.onTools(catalog, pending, changed)
}

func (c *connections) shutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("mcp: shutdown context is required")
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		for _, configuredServer := range c.servers {
			if configuredServer.attempt != nil {
				configuredServer.attempt.cancel()
				configuredServer.attempt = nil
			}
		}
		c.servers = nil
		c.publishToolsLocked()
	}
	attempt := c.shutdownState
	if attempt == nil {
		attempt = &shutdownAttempt{done: make(chan struct{})}
		c.shutdownState = attempt
		go c.closeAll(attempt)
	}
	c.mu.Unlock()

	if err := completion.Wait(ctx, attempt.done); err != nil {
		return err
	}
	return attempt.err
}

func (c *connections) closeAll(attempt *shutdownAttempt) {
	c.attempts.Wait()

	c.mu.Lock()
	closeAttempts := make([]*sessionCloseAttempt, 0, len(c.sessions)+len(c.retirements))
	seen := make(map[*sessionCloseAttempt]struct{}, len(c.sessions)+len(c.retirements))
	for session := range c.sessions {
		closeAttempt := c.beginSessionCloseLocked(session)
		if closeAttempt != nil {
			if _, ok := seen[closeAttempt]; !ok {
				seen[closeAttempt] = struct{}{}
				closeAttempts = append(closeAttempts, closeAttempt)
			}
		}
	}
	for closeAttempt := range c.retirements {
		if _, ok := seen[closeAttempt]; !ok {
			seen[closeAttempt] = struct{}{}
			closeAttempts = append(closeAttempts, closeAttempt)
		}
	}
	c.mu.Unlock()

	var errs []error
	for _, closeAttempt := range closeAttempts {
		<-closeAttempt.done
		if closeAttempt.err != nil {
			errs = append(errs, closeAttempt.err)
		}
	}
	c.observers.Wait()

	c.mu.Lock()
	for closeAttempt := range seen {
		delete(c.retirements, closeAttempt)
	}
	attempt.err = errors.Join(errs...)
	close(attempt.done)
	c.mu.Unlock()
}

func (c *connections) closeSession(ctx context.Context, session *sdk.ClientSession) error {
	if session == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("mcp: session close context is required")
	}
	attempt := c.beginSessionClose(session)
	if attempt == nil {
		return nil
	}

	if err := completion.Wait(ctx, attempt.done); err != nil {
		c.mu.Lock()
		select {
		case <-attempt.done:
			c.mu.Unlock()
			return attempt.err
		default:
			if c.retirements == nil {
				c.retirements = make(map[*sessionCloseAttempt]struct{})
			}
			c.retirements[attempt] = struct{}{}
		}
		c.mu.Unlock()
		return err
	}
	return attempt.err
}

func (c *connections) beginSessionClose(session *sdk.ClientSession) *sessionCloseAttempt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.beginSessionCloseLocked(session)
}

func (c *connections) beginSessionCloseLocked(session *sdk.ClientSession) *sessionCloseAttempt {
	owned := c.sessions[session]
	if owned == nil {
		return nil
	}
	attempt := owned.close
	if attempt == nil {
		attempt = &sessionCloseAttempt{done: make(chan struct{})}
		owned.close = attempt
		if owned.cancelTools != nil {
			owned.cancelTools()
		}
		go c.closeSessionAttempt(session, owned, attempt)
	}
	return attempt
}

func closeOwnedSession(owned *ownedSession) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mcp: session close panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return owned.closeFn()
}

func (c *connections) closeSessionAttempt(
	session *sdk.ClientSession,
	owned *ownedSession,
	attempt *sessionCloseAttempt,
) {
	err := closeOwnedSession(owned)
	if err != nil {
		slog.ErrorContext(c.lifetime, "mcp: session retirement failed", telemetry.Error(err))
	}
	c.mu.Lock()
	if current := c.sessions[session]; current == owned && current.close == attempt {
		delete(c.sessions, session)
	}
	if err == nil {
		delete(c.retirements, attempt)
	}
	attempt.err = err
	close(attempt.done)
	c.mu.Unlock()
}
