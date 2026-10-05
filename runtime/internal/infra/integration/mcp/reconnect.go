package mcp

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/go-sdk/auth"
)

// Configure adds a new server or re-dials an existing one with the given
// config, then refreshes the model-facing tool set so the model immediately
// sees the (re)connected server. It is the runtime-mutable counterpart to the
// boot-time [Dial]: create, update, enable and reconnect obtain the current
// owner configuration before entering this connection attempt.
//
// cfg was read before this call, so a detach or newer operation may already
// have superseded it. The owner cancels ctx before it detaches a server; the
// check under mu therefore guarantees a superseded configuration never
// re-adds a detached server or replaces a newer connection.
func (c *Connections) Configure(ctx context.Context, cfg ServerConfig) error {
	cfg = cfg.Clone()
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("mcp: invalid server %q: %w", cfg.ID(), err)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return err
	}
	configuredServer := c.find(cfg.ID())
	if configuredServer == nil {
		configuredServer = &server{id: cfg.ID(), config: cfg}
		c.servers = append(c.servers, configuredServer)
	}
	oauth := reusableOAuth(configuredServer.config, cfg, configuredServer.oauth)
	if oauth == nil {
		oauth = cfg.OAuthHandler
	}
	configuredServer.oauth = oauth
	detachedSession := configuredServer.session
	configuredServer.config = cfg
	configuredServer.config.OAuthHandler = nil
	configuredServer.settle(nil, nil, mcpserver.ConnectionConnecting, "")
	cfg.OAuthHandler = oauth
	attempt := c.beginAttempt(ctx, configuredServer)
	c.publishToolsLocked()
	c.mu.Unlock()
	defer c.finishAttempt(attempt)

	closeErr := c.closeSession(attempt.ctx, detachedSession)
	if cfg.Transport == TransportHTTP && oauth == nil && cfg.Authorization == "" {
		var err error
		oauth, err = restoreOAuthHandler(attempt.ctx, c.lifetime, c.oauthSessions, cfg.oauthTarget())
		if err != nil {
			c.failAttempt(attempt, mcpserver.FailureConfiguration)
			return errors.Join(closeErr, err)
		}
		c.mu.Lock()
		if c.closed || !c.currentAttempt(attempt) {
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return errors.Join(closeErr, ErrConnectionsClosed)
			}
			return errors.Join(closeErr, errConnectionSuperseded)
		}
		configuredServer.oauth = oauth
		c.mu.Unlock()
		cfg.OAuthHandler = oauth
	}
	return errors.Join(closeErr, c.dialAndSwap(attempt, cfg, false))
}

func reusableOAuth(current, candidate ServerConfig, handler auth.OAuthHandler) auth.OAuthHandler {
	if handler == nil ||
		current.Transport != TransportHTTP ||
		candidate.Transport != TransportHTTP ||
		candidate.Authorization != "" ||
		current.oauthTarget().Fingerprint() != candidate.oauthTarget().Fingerprint() {
		return nil
	}
	return handler
}

// Authorize runs the interactive OAuth sign-in for an HTTP server: it opens the
// system browser to the authorization URL, catches the redirect on a loopback
// callback, and (via the go-sdk) discovers + dynamically registers + exchanges
// the code. On success the live OAuth handler is kept on the server (reused by
// later reconnects this session, auto-refreshing) and the server connects. The
// handler and its refreshing token source are persisted when a session store
// is configured. Blocks until the user completes the browser flow or
// [oauthFlowTimeout] elapses. Returns
// [mcpserver.ErrUnknownServer] for an unconfigured name. A newer operation for
// the same server supersedes this attempt.
func (c *Connections) Authorize(ctx context.Context, input ServerConfig) (err error) {
	input = input.Clone()
	if err := input.Validate(); err != nil {
		return err
	}
	if input.OAuthHandler != nil {
		return errors.New("mcp: explicit authorization cannot supply an OAuth handler")
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	configuredServer := c.find(input.ID())
	if configuredServer == nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: %q", mcpserver.ErrUnknownServer, input.ID())
	}
	if input.Transport != TransportHTTP {
		c.mu.Unlock()
		return errors.New("mcp: OAuth applies to HTTP servers only")
	}
	if input.Authorization != "" {
		c.mu.Unlock()
		return errors.New("mcp: clear static authorization before starting OAuth")
	}
	detachedSession := configuredServer.session
	configuredServer.config = input.Clone()
	configuredServer.oauth = nil
	configuredServer.settle(nil, nil, mcpserver.ConnectionConnecting, "")
	attempt := c.beginAttempt(ctx, configuredServer)
	cfg := input
	c.publishToolsLocked()
	c.mu.Unlock()
	defer c.finishAttempt(attempt)

	closeErr := c.closeSession(attempt.ctx, detachedSession)

	// Bound the human-in-the-loop flow here; clear the per-server handshake
	// timeout so it can't abort the browser wait mid-sign-in.
	ctx, cancel := context.WithTimeout(attempt.ctx, oauthFlowTimeout)
	defer cancel()
	cfg.HandshakeTimeout = nil

	flow, err := newOAuthFlow(ctx)
	if err != nil {
		c.failAttempt(attempt, mcpserver.FailureAuthorization)
		return errors.Join(closeErr, err)
	}
	defer func() { err = errors.Join(err, flow.close(ctx)) }()
	handler, err := newOAuthHandler(ctx, flow, c.lifetime, c.oauthSessions, cfg.oauthTarget())
	if err != nil {
		c.failAttempt(attempt, mcpserver.FailureAuthorization)
		return errors.Join(closeErr, err)
	}
	cfg.OAuthHandler = handler

	attempt.ctx = ctx
	return errors.Join(closeErr, c.dialAndSwap(attempt, cfg, true))
}

// dialAndSwap dials cfg, proves the session with a tools/list, then publishes it
// on the configured server under the lock — the shared tail of
// [Connections.Configure] / [Connections.Authorize]. The per-server attempt
// registration rejects a stale completion after a newer
// configure/remove/reconnect. c.mu is not held while dialing. keepHandler stores
// cfg.OAuthHandler on that server after a successful connect (Authorize keeps the
// just-authorized handler for this session's later reconnects; the plain dials
// reuse an existing one and pass false).
func (c *Connections) dialAndSwap(attempt *connectionAttempt, cfg ServerConfig, keepHandler bool) error {
	session, cleanupSession, err := dial(attempt.ctx, c.lifetime, c.client, cfg)
	step := mcpserver.FailureConnection
	var verifiedTools []Executable
	if err == nil {
		// Prove the session is usable before publishing it as connected.
		step = mcpserver.FailureToolDiscovery
		verifiedTools, err = sourceTools(attempt.ctx, c, cfg, session)
	}

	c.mu.Lock()
	c.ownSessionLocked(session, cleanupSession)
	current := c.currentAttempt(attempt)
	closed := c.closed
	if closed || !current {
		// Shutdown ran while we were dialing outside the lock: it niled c.servers
		// (so this server is detached) and closed every session. Storing the fresh
		// session here would strand it past Shutdown's sweep — a connection leak.
		// Drop it instead. Mirrors lsp.Servers.clientFor's closed re-check.
		c.mu.Unlock()
		closeErr := c.closeSession(attempt.ctx, session)
		if closed {
			return errors.Join(ErrConnectionsClosed, err, closeErr)
		}
		return errors.Join(errConnectionSuperseded, err, closeErr)
	}
	if err != nil {
		state, failure := failedStatus(err, step)
		attempt.target.settle(nil, nil, state, failure)
		if state == mcpserver.ConnectionNeedsAuth {
			attempt.target.oauth = nil
		}
	} else {
		attempt.target.settle(session, verifiedTools, mcpserver.ConnectionConnected, "")
		if keepHandler {
			attempt.target.oauth = cfg.OAuthHandler // keep the authorized handler for this session's reconnects
		}
	}
	attempt.target.attempt = nil
	// Publish only the snapshots proved above, in the critical section that
	// settled them. Re-querying every other server here would let an unrelated
	// transient tools/list failure or cancellation silently erase its tools
	// while its status remained connected.
	c.publishToolsLocked()
	// A session that dialed OK but then failed verification or lost the collision
	// race is now detached (target.session was set nil above). Close it after
	// releasing c.mu — never hold the lock across a session teardown.
	staleSession := session
	if err == nil {
		staleSession = nil
	}
	c.mu.Unlock()

	if staleSession != nil {
		err = errors.Join(err, c.closeSession(attempt.ctx, staleSession))
	}
	return err
}

var errConnectionSuperseded = errors.New("mcp: connection operation superseded")

// Refuse records that the configuration owner refused a new connection for
// name. The refusal is that server's latest operation: it supersedes an
// in-flight attempt, withdraws the session and its tools, and settles the
// failure category, so a refused source never keeps serving through the
// session it had before. ctx belongs to the refused operation; once that
// operation has been superseded it records nothing.
func (c *Connections) Refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error {
	if err := name.Validate(); err != nil {
		return fmt.Errorf("mcp: refused server: %w", err)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrConnectionsClosed
	}
	if err := ctx.Err(); err != nil {
		c.mu.Unlock()
		return err
	}
	target := c.find(name)
	if target == nil {
		target = &server{id: name}
		c.servers = append(c.servers, target)
	}
	if target.attempt != nil {
		target.attempt.cancel()
		target.attempt = nil
	}
	withdrawn := target.session
	target.settle(nil, nil, mcpserver.ConnectionFailed, failure)
	closeAttempt := c.beginSessionCloseLocked(withdrawn)
	if closeAttempt != nil {
		if c.retirements == nil {
			c.retirements = map[*sessionCloseAttempt]struct{}{}
		}
		c.retirements[closeAttempt] = struct{}{}
	}
	c.publishToolsLocked()
	c.mu.Unlock()
	return nil
}

type connectionAttempt struct {
	target *server
	ctx    context.Context
	cancel context.CancelFunc
}

// beginAttempt is called with c.mu held. It cancels only the previous operation
// for this server; unrelated servers continue independently.
func (c *Connections) beginAttempt(parent context.Context, target *server) *connectionAttempt {
	if target.attempt != nil {
		target.attempt.cancel()
	}
	ctx, cancel := context.WithCancel(parent)
	attempt := &connectionAttempt{target: target, ctx: ctx, cancel: cancel}
	target.attempt = attempt
	c.attempts.Add(1)
	return attempt
}

func (c *Connections) finishAttempt(attempt *connectionAttempt) {
	c.mu.Lock()
	if c.currentAttempt(attempt) {
		attempt.target.attempt = nil
	}
	c.mu.Unlock()
	attempt.cancel()
	c.attempts.Done()
}

// currentAttempt is called with c.mu held.
func (c *Connections) currentAttempt(attempt *connectionAttempt) bool {
	return c.find(attempt.target.name()) == attempt.target &&
		attempt.target.attempt == attempt
}

func (c *Connections) failAttempt(attempt *connectionAttempt, failure mcpserver.ConnectionFailure) {
	c.mu.Lock()
	if c.currentAttempt(attempt) {
		attempt.target.settle(nil, nil, mcpserver.ConnectionFailed, failure)
		attempt.target.attempt = nil
	}
	c.mu.Unlock()
}
