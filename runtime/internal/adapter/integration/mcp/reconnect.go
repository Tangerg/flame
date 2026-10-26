package mcp

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/go-sdk/auth"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func (c *connections) configure(attempt *attempt, prepared *launch) error {
	cfg := &prepared.config
	c.mu.Lock()
	if err := c.cancellationErr(attempt.ctx); !c.currentAttempt(attempt) || err != nil {
		c.mu.Unlock()
		return errors.Join(errConnectionSuperseded, err)
	}
	target := attempt.target
	oauth := reusableOAuth(target.config, *cfg, target.oauth)
	if oauth == nil {
		oauth = cfg.OAuthHandler
	}
	target.oauth = oauth
	target.config = cfg.Clone()
	target.config.OAuthHandler = nil
	cfg.OAuthHandler = oauth
	c.mu.Unlock()

	if cfg.Transport == mcpserver.TransportStreamableHTTP && oauth == nil && cfg.Authorization == "" {
		var err error
		oauth, err = restoreOAuthHandler(attempt.ctx, c.lifetime, c.oauthSessions, cfg.OAuthTarget())
		if err != nil {
			return err
		}
		c.mu.Lock()
		if err := c.cancellationErr(attempt.ctx); !c.currentAttempt(attempt) || err != nil {
			c.mu.Unlock()
			return errors.Join(errConnectionSuperseded, err)
		}
		target.oauth = oauth
		c.mu.Unlock()
		cfg.OAuthHandler = oauth
	}
	return c.dialAndSwap(attempt.ctx, attempt, prepared, false)
}

func reusableOAuth(current, candidate ServerConfig, handler auth.OAuthHandler) auth.OAuthHandler {
	if handler == nil ||
		current.Transport != mcpserver.TransportStreamableHTTP ||
		candidate.Transport != mcpserver.TransportStreamableHTTP ||
		candidate.Authorization != "" ||
		current.OAuthTarget().Fingerprint() != candidate.OAuthTarget().Fingerprint() {
		return nil
	}
	return handler
}

func (c *connections) authorize(attempt *attempt, prepared *launch) (err error) {
	cfg := &prepared.config
	if cfg.OAuthHandler != nil {
		return errors.New("mcp: explicit authorization cannot supply an OAuth handler")
	}
	if cfg.Transport != mcpserver.TransportStreamableHTTP {
		return errors.New("mcp: OAuth applies to HTTP servers only")
	}
	if cfg.Authorization != "" {
		return errors.New("mcp: clear static authorization before starting OAuth")
	}
	c.mu.Lock()
	if err := c.cancellationErr(attempt.ctx); !c.currentAttempt(attempt) || err != nil {
		c.mu.Unlock()
		return errors.Join(errConnectionSuperseded, err)
	}
	attempt.target.config = cfg.Clone()
	attempt.target.oauth = nil
	c.mu.Unlock()

	// The human flow has its own deadline; a handshake timeout cannot bound it.
	ctx, cancel := context.WithTimeout(attempt.ctx, oauthFlowTimeout)
	defer cancel()
	cfg.HandshakeTimeout = mcpserver.HandshakeTimeout{}
	flow, err := newOAuthFlow(ctx)
	if err != nil {
		c.failAttempt(attempt, mcpserver.FailureAuthorization)
		return err
	}
	defer func() { err = errors.Join(err, flow.close(ctx)) }()
	handler, err := newOAuthHandler(ctx, flow, c.lifetime, c.oauthSessions, cfg.OAuthTarget())
	if err != nil {
		c.failAttempt(attempt, mcpserver.FailureAuthorization)
		return err
	}
	cfg.OAuthHandler = handler
	return c.dialAndSwap(ctx, attempt, prepared, true)
}

// dialAndSwap publishes a session only after tool discovery, while its attempt
// still owns the server. Authorization retains the handler for later reconnects.
func (c *connections) dialAndSwap(ctx context.Context, attempt *attempt, prepared *launch, keepHandler bool) error {
	cfg := prepared.config
	session, cleanupSession, err := dial(ctx, c.lifetime, c.client, prepared)
	step := mcpserver.FailureConnection
	var verifiedTools []Executable
	c.mu.Lock()
	c.ownSessionLocked(session, cleanupSession, prepared.admission.Bindings)
	c.mu.Unlock()
	if err == nil && prepared.admission.Run != nil {
		step = mcpserver.FailureConfiguration
		admissionCtx := ctx
		if duration, bounded := cfg.HandshakeTimeout.Duration(); bounded {
			var cancel context.CancelFunc
			admissionCtx, cancel = context.WithTimeout(ctx, duration)
			defer cancel()
		}
		err = admitSession(admissionCtx, session, prepared.admission.Run)
	}
	if err == nil {
		// Prove the session is usable before publishing it as connected.
		step = mcpserver.FailureToolDiscovery
		verifiedTools, err = sourceTools(ctx, c, cfg, session)
	}
	c.mu.Lock()
	// Authorization may expire while waiting for the publication lock.
	err = errors.Join(err, c.cancellationErr(ctx))
	current := c.currentAttempt(attempt) && c.cancellationErr(attempt.ctx) == nil
	closed := c.closed
	if closed || !current {
		// Shutdown ran while we were dialing outside the lock: it niled c.servers
		// (so this server is detached) and closed every session. Storing the fresh
		// session here would strand it past Shutdown's sweep — a connection leak.
		// The unpublished session stays in the retirement ledger.
		c.mu.Unlock()
		closeErr := c.closeSession(attempt.ctx, session)
		if closed {
			return errors.Join(errConnectionsClosed, c.cancellationErr(attempt.ctx), err, closeErr)
		}
		return errors.Join(errConnectionSuperseded, c.cancellationErr(attempt.ctx), err, closeErr)
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
	c.publishToolsLocked(attempt.target.id)
	// Failed verification withdraws the session. Teardown must run outside mu.
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

func admitSession(ctx context.Context, session *sdk.ClientSession, admit func(context.Context, *sdk.ClientSession) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mcp: connection admission panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return errors.Join(admit(ctx, session), cancellation.Err(ctx))
}

var errConnectionSuperseded = errors.New("mcp: connection operation superseded")

// Refuse records that the configuration owner refused a new connection for
// name. The refusal is that server's latest operation: it supersedes an
// in-flight attempt, withdraws the session and its tools, and settles the
// failure category, so a refused source never keeps serving through the
// session it had before. ctx belongs to the refused operation; once that
// operation has been superseded it records nothing.
func (c *connections) refuse(ctx context.Context, name mcpserver.ID, failure mcpserver.ConnectionFailure) error {
	if err := name.Validate(); err != nil {
		return fmt.Errorf("mcp: refused server: %w", err)
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errConnectionsClosed
	}
	if err := c.cancellationErr(ctx); err != nil {
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
	c.retireSessionLocked(withdrawn)
	c.publishToolsLocked(name)
	c.mu.Unlock()
	return nil
}
