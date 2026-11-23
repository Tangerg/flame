package mcp

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

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

	if grant, ok := cfg.OAuthTarget(); ok && oauth == nil {
		var err error
		oauth, err = restoreOAuthHandler(attempt.ctx, c.lifetime, c.oauthSessions, grant)
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
	return c.dialAndSwap(attempt.ctx, attempt, prepared)
}

func reusableOAuth(current, candidate ServerConfig, handler auth.OAuthHandler) auth.OAuthHandler {
	currentTarget, currentOK := current.OAuthTarget()
	candidateTarget, candidateOK := candidate.OAuthTarget()
	if handler == nil || !currentOK || !candidateOK || currentTarget.Fingerprint() != candidateTarget.Fingerprint() {
		return nil
	}
	return handler
}

func (c *connections) authorize(attempt *attempt, prepared *launch) (err error) {
	cfg := &prepared.config
	if cfg.OAuthHandler != nil {
		return errors.New("mcp: explicit authorization cannot supply an OAuth handler")
	}
	target, ok := cfg.OAuthTarget()
	if !ok {
		return errors.New("mcp: OAuth applies to HTTP servers without static authorization")
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
		c.failAttempt(attempt, mcpserver.FailureAuthorization, err)
		return err
	}
	defer func() { err = errors.Join(err, flow.close(ctx)) }()
	handler, err := newOAuthHandler(ctx, flow, c.lifetime, c.oauthSessions, target)
	if err != nil {
		c.failAttempt(attempt, mcpserver.FailureAuthorization, err)
		return err
	}
	cfg.OAuthHandler = handler
	return c.dialAndSwap(ctx, attempt, prepared)
}

func (c *connections) adoptLaunchSession(ctx context.Context, prepared *launch) (*sdk.ClientSession, *ownedSession, uint64, error) {
	var session *sdk.ClientSession
	var cleanupSession sessionCleanup
	var observation *installedObservation
	var err error
	if prepared.managed != nil {
		observation = prepared.managed.observe()
		session, cleanupSession = observation.session, observation.retire
		prepared.retire = func() error { return prepared.managed.release(context.WithoutCancel(ctx)) }
	} else {
		session, cleanupSession, err = dial(ctx, c.lifetime, c.client, prepared)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if prepared.managed == nil {
		err = errors.Join(err, c.ownSessionLocked(session, cleanupSession, prepared.admission.Bindings))
	} else if session != nil {
		c.sessions[session] = &ownedSession{closeFn: cleanupSession, bindings: prepared.admission.Bindings, managed: observation}
	}
	owned := c.sessions[session]
	var toolVersion uint64
	if owned != nil {
		toolVersion = owned.toolVersion
	}
	return session, owned, toolVersion, err
}

func (c *connections) dialAndSwap(ctx context.Context, attempt *attempt, prepared *launch) error {
	cfg := prepared.config
	session, owned, toolVersion, err := c.adoptLaunchSession(ctx, prepared)
	step := mcpserver.FailureConnection
	var verifiedTools []Executable
	if err == nil {
		verifiedTools, step, err = c.proveSession(ctx, cfg, session, prepared.admission.Run)
	}
	if prepared.managed != nil {
		err = errors.Join(err, prepared.close())
	}
	c.mu.Lock()
	if owned != nil && owned.end != nil {
		select {
		case <-owned.end.done:
			err = errors.Join(err, errSessionEnded, owned.end.err)
			step = mcpserver.FailureConnection
		default:
		}
	}
	// Authorization may expire while waiting for the publication lock.
	err = errors.Join(err, c.cancellationErr(ctx))
	current := c.currentAttempt(attempt) && c.cancellationErr(attempt.ctx) == nil
	closed := c.closed
	if closed || !current {
		// Shutdown ran while dialing outside the lock; storing the fresh session would
		// strand it past Shutdown's sweep.
		c.mu.Unlock()
		closeErr := c.closeSession(attempt.ctx, session)
		if closed {
			return errors.Join(errConnectionsClosed, c.cancellationErr(attempt.ctx), err, closeErr)
		}
		return errors.Join(errConnectionSuperseded, c.cancellationErr(attempt.ctx), err, closeErr)
	}
	if err != nil {
		attempt.target.fail(err, step)
	} else {
		c.connectLocked(attempt, session, owned, cfg, verifiedTools, toolVersion)
	}
	attempt.target.attempt = nil
	// Publish only the snapshots proved above: re-querying other servers would let an
	// unrelated transient tools/list failure erase their tools while their status
	// stays connected.
	c.publishToolsLocked(attempt.target.id)
	// Teardown must run outside mu.
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

func (c *connections) connectLocked(attempt *attempt, session *sdk.ClientSession, owned *ownedSession, cfg ServerConfig, tools []Executable, toolVersion uint64) {
	target := attempt.target
	for index := range tools {
		tools[index].catalogVersion = toolVersion
	}
	target.markConnected(session, tools)
	if owned != nil && owned.toolChanges != nil {
		c.watchToolChangesLocked(session, owned, cfg, toolVersion)
	}
	if attempt.purpose == attemptAuthorize {
		target.oauth = cfg.OAuthHandler
	}
}

func (c *connections) proveSession(ctx context.Context, cfg ServerConfig, session *sdk.ClientSession, admit func(context.Context, *sdk.ClientSession) error) ([]Executable, mcpserver.ConnectionFailure, error) {
	if admit != nil {
		admissionCtx := ctx
		if duration, bounded := cfg.HandshakeTimeout.Duration(); bounded {
			var cancel context.CancelFunc
			admissionCtx, cancel = context.WithTimeout(ctx, duration)
			defer cancel()
		}
		if err := admitSession(admissionCtx, session, admit); err != nil {
			return nil, mcpserver.FailureConfiguration, err
		}
	}
	tools, err := sourceTools(ctx, c, cfg, session)
	return tools, mcpserver.FailureToolDiscovery, err
}

func admitSession(ctx context.Context, session *sdk.ClientSession, admit func(context.Context, *sdk.ClientSession) error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mcp: connection admission panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return errors.Join(admit(ctx, session), errors.Join(ctx.Err(), context.Cause(ctx)))
}

var errConnectionSuperseded = errors.New("mcp: connection operation superseded")

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
	target.markFailed(failure, nil)
	c.retireSessionLocked(withdrawn)
	c.publishToolsLocked(name)
	c.mu.Unlock()
	return nil
}
