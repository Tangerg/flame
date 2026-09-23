package mcp

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"

	toolcontract "github.com/Tangerg/scope/core/tool"

	sdkmcp "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// Statuses returns one cached entry per server attached to the live projection
// (connected and failed alike), in dial order.
func (c *Connections) Statuses() []mcpserver.ConnectionStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]mcpserver.ConnectionStatus, 0, len(c.servers))
	for _, configuredServer := range c.servers {
		out = append(out, mcpserver.ConnectionStatus{
			Name:      configuredServer.name(),
			State:     configuredServer.state,
			ToolCount: len(configuredServer.tools),
		})
	}
	return out
}

// Tools projects the same admitted Scope snapshot used by execution and status.
// Remote changes take effect when reconnect admits a replacement catalog, so
// this read performs no remote discovery.
func (c *Connections) Tools(serverName *mcpserver.ServerName) ([]mcpserver.AdvertisedTool, error) {
	c.mu.Lock()
	var catalog []toolcontract.Tool
	for _, configuredServer := range c.servers {
		if configuredServer.session != nil && (serverName == nil || configuredServer.name() == *serverName) {
			catalog = append(catalog, configuredServer.tools...)
		}
	}
	c.mu.Unlock()

	out := make([]mcpserver.AdvertisedTool, 0, len(catalog))
	for _, executable := range catalog {
		ref, found, err := IdentifyTool(executable)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, errors.New("mcp: admitted tool has no MCP identity")
		}
		out = append(out, mcpserver.AdvertisedTool{
			Server: ref.Server, Name: ref.Tool, Definition: executable.Definition(),
		})
	}
	return out, nil
}

// Detach removes a server from the live projection and starts retiring its
// session. Session teardown remains owned by Connections and is joined by
// Shutdown; it never delays the application control-plane mutation.
func (c *Connections) Detach(name mcpserver.ServerName) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	var detachedSession *sdkmcp.ClientSession
	if index := slices.IndexFunc(c.servers, func(configuredServer *server) bool { return configuredServer.name() == name }); index >= 0 {
		target := c.servers[index]
		detachedSession = target.session
		if target.attempt != nil {
			target.attempt.cancel()
			target.attempt = nil
		}
		// slices.Delete clears the vacated pointer, so the long-lived backing
		// array cannot retain the removed session and its verified tool wrappers.
		c.servers = slices.Delete(c.servers, index, index+1)
	}
	c.mu.Unlock()

	// Shrink the model-facing catalog before a potentially-blocking session
	// close. The publication lock keeps this ordered with every dial.
	c.publishTools()
	c.retireSession(detachedSession)
	return nil
}

func (c *Connections) retireSession(session *sdkmcp.ClientSession) {
	if session == nil {
		return
	}
	c.mu.Lock()
	attempt := c.beginSessionCloseLocked(session)
	if attempt != nil {
		if c.retirements == nil {
			c.retirements = make(map[*sessionCloseAttempt]struct{})
		}
		c.retirements[attempt] = struct{}{}
	}
	c.mu.Unlock()
}

// publishTools rebuilds the model-facing catalog from each connected server's
// last verified tool snapshot. Network I/O happens only while establishing that
// server's session; publication itself is deterministic and cannot turn caller
// cancellation or another server's independent failure into a false catalog.
func (c *Connections) publishTools() {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	c.mu.Lock()
	var catalog []toolcontract.Tool
	for _, configuredServer := range c.servers {
		if configuredServer.session != nil {
			catalog = append(catalog, configuredServer.tools...)
		}
	}
	sink := c.onTools
	c.mu.Unlock()

	if sink != nil {
		sink(catalog)
	}
}

// Shutdown rejects new operations, joins all admitted dials, and closes every
// session still present in the ownership ledger. ClientSession.Close consumes
// its transport closer even when it returns an error, so that diagnostic is
// terminal and the session leaves the ledger after the attempt completes.
func (c *Connections) Shutdown(ctx context.Context) error {
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
	}
	attempt := c.shutdown
	if attempt != nil {
		select {
		case <-attempt.done:
			// Every session close owned by the completed shutdown attempt reached its
			// terminal state. Its diagnostic was reported to callers that joined
			// that attempt; repeating Shutdown is an idempotent no-op.
			c.mu.Unlock()
			return nil
		default:
		}
	}
	if attempt == nil {
		attempt = &shutdownAttempt{done: make(chan struct{})}
		c.shutdown = attempt
		go c.closeAll(attempt)
	}
	c.mu.Unlock()

	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Connections) closeAll(attempt *shutdownAttempt) {
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

	c.mu.Lock()
	// A racing Detach may register an attempt after the initial snapshot, but
	// the session itself guaranteed that attempt was already in closeAttempts.
	// Consume the diagnostic only after the joined attempt has completed.
	for closeAttempt := range seen {
		delete(c.retirements, closeAttempt)
	}
	attempt.err = errors.Join(errs...)
	close(attempt.done)
	c.mu.Unlock()
}

func (c *Connections) closeSession(ctx context.Context, session *sdkmcp.ClientSession) error {
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

	select {
	case <-attempt.done:
		return attempt.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Connections) beginSessionClose(session *sdkmcp.ClientSession) *sessionCloseAttempt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.beginSessionCloseLocked(session)
}

func (c *Connections) beginSessionCloseLocked(session *sdkmcp.ClientSession) *sessionCloseAttempt {
	owned := c.sessions[session]
	if owned == nil {
		return nil
	}
	attempt := owned.close
	if attempt == nil {
		attempt = &sessionCloseAttempt{done: make(chan struct{})}
		owned.close = attempt
		go c.closeSessionAttempt(session, owned, attempt)
	}
	return attempt
}

// closeOwnedSession keeps a defect inside an external session's closer from
// ending the process and stranding every Shutdown waiting on this attempt. A
// close that fails already lands in the attempt's error; a close that panics is
// one of those.
func closeOwnedSession(owned *ownedSession) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mcp: session close panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return owned.closeFn()
}

func (c *Connections) closeSessionAttempt(
	session *sdkmcp.ClientSession,
	owned *ownedSession,
	attempt *sessionCloseAttempt,
) {
	err := closeOwnedSession(owned)
	c.mu.Lock()
	if current := c.sessions[session]; current == owned && current.close == attempt {
		// sdkmcp.ClientSession.Close is one-shot: its underlying transport
		// closer is consumed before the error returns. Retaining this entry
		// would only replay a cached diagnostic, never advance cleanup.
		delete(c.sessions, session)
	}
	if err == nil {
		// Successful asynchronous retirement has no diagnostic to preserve for
		// Shutdown; a failed attempt stays in retirements until it is reported.
		delete(c.retirements, attempt)
	}
	attempt.err = err
	close(attempt.done)
	c.mu.Unlock()
}
