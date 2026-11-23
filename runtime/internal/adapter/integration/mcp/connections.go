package mcp

import (
	"context"
	"errors"
	"os/exec"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/go-sdk/auth"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

var errConnectionsClosed = errors.New("mcp: connections closed")

type server struct {
	id      mcpserver.ID
	config  ServerConfig
	session *sdk.ClientSession
	tools   []Executable
	state   mcpserver.ConnectionState
	failure mcpserver.ConnectionFailure
	cause   error

	oauth auth.OAuthHandler

	attempt *attempt
}

func (s *server) name() mcpserver.ID { return s.id }

func (s *server) markConnecting() {
	s.session, s.tools, s.state, s.failure, s.cause = nil, nil, mcpserver.ConnectionConnecting, "", nil
}

func (s *server) markConnected(session *sdk.ClientSession, tools []Executable) {
	s.session, s.tools, s.state, s.failure, s.cause = session, tools, mcpserver.ConnectionConnected, "", nil
}

func (s *server) markFailed(failure mcpserver.ConnectionFailure, cause error) {
	s.session, s.tools, s.state, s.failure, s.cause = nil, nil, mcpserver.ConnectionFailed, failure, cause
}

// A server that requires authorization is waiting for the user, not failing,
// and its stale OAuth handler must not be reused.
func (s *server) fail(cause error, step mcpserver.ConnectionFailure) {
	if !errors.Is(cause, mcpserver.ErrAuthorizationRequired) {
		s.markFailed(step, cause)
		return
	}
	s.session, s.tools, s.state, s.failure, s.cause = nil, nil, mcpserver.ConnectionNeedsAuth, "", nil
	s.oauth = nil
}

type shutdownAttempt struct {
	done chan struct{}
	err  error
}

type sessionCloseAttempt struct {
	done chan struct{}
	err  error
}

type ownedSession struct {
	closeFn     func() error
	managed     *installedObservation
	close       *sessionCloseAttempt
	bindings    *plugin.RequiredBindings
	end         *sessionEnd
	toolChanges chan struct{}
	toolVersion uint64
	cancelTools context.CancelFunc
}

type sessionEnd struct {
	done chan struct{}
	err  error
}

type connections struct {
	lifetime  context.Context
	callbacks *pluginmcp.Callbacks
	mu        sync.Mutex
	servers   []*server
	client    *sdk.Client
	// onTools runs while mu is held, so it must be an in-memory publication that never
	// blocks or calls back into connections.
	onTools       func([]Executable, []mcpserver.ID, []mcpserver.ID)
	closed        bool
	shutdownState *shutdownAttempt
	sessions      map[*sdk.ClientSession]*ownedSession
	retirements   map[*sessionCloseAttempt]struct{}

	oauthSessions oauthSessionStore

	attempts  sync.WaitGroup
	observers sync.WaitGroup
}

func (c *connections) cancellationErr(ctx context.Context) error {
	return errors.Join(errors.Join(ctx.Err(), context.Cause(ctx)), errors.Join(c.lifetime.Err(), context.Cause(c.lifetime)))
}

func (c *connections) setToolSink(sink func([]Executable, []mcpserver.ID, []mcpserver.ID)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onTools = sink
	c.publishToolsLocked()
}

func clientImplementation() *sdk.Implementation {
	return &sdk.Implementation{Name: identity.ProductName, Version: clientProtocolVersion}
}

const clientProtocolVersion = "v0.1.0"

func newClient(changed func(context.Context, *sdk.ToolListChangedRequest)) *sdk.Client {
	return sdk.NewClient(clientImplementation(), &sdk.ClientOptions{ToolListChangedHandler: changed})
}

func (c *connections) find(name mcpserver.ID) *server {
	for _, configuredServer := range c.servers {
		if configuredServer.name() == name {
			return configuredServer
		}
	}
	return nil
}

func (c *connections) current(name mcpserver.ID, session *sdk.ClientSession, toolVersion uint64) bool {
	if session == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	configuredServer := c.find(name)
	owned := c.sessions[session]
	return configuredServer != nil && configuredServer.session == session && owned != nil && owned.close == nil && owned.toolVersion == toolVersion
}

func (c *connections) ownSessionLocked(session *sdk.ClientSession, cleanup sessionCleanup, bindings *plugin.RequiredBindings) error {
	if session == nil {
		return nil
	}
	var err error
	if c.sessions[session] == nil {
		if c.callbacks != nil {
			err = c.callbacks.AdmitSession(session)
		}
		owned := &ownedSession{bindings: bindings, toolChanges: make(chan struct{}, 1), end: &sessionEnd{done: make(chan struct{})}, closeFn: func() error {
			if c.callbacks != nil {
				c.callbacks.RevokeSession(session)
			}
			return retireSession(session, cleanup)
		}}
		c.sessions[session] = owned
		c.observers.Add(1)
		go c.observeSessionEnd(session, owned)
	}
	return err
}

func retireSession(session *sdk.ClientSession, cleanup sessionCleanup) (err error) {
	defer func() { err = errors.Join(err, cleanup()) }()
	err = session.Close()
	// A canceled command that exits cleanly makes exec.Cmd.Wait return
	// context.Canceled. Cleanup still verifies process-group termination.
	if errors.Is(err, context.Canceled) {
		err = nil
	}
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		err = nil
	}
	return err
}

var ErrUnavailable = errors.New("mcp: exact backend connection is unavailable")

// Re-reading current selections would retarget a waiting execution after a
// collaborator changed its own selection.
func (c *connections) activation(session *sdk.ClientSession) (plugin.Activation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	owned := c.sessions[session]
	if c.closed || owned == nil || owned.close != nil || owned.managed == nil {
		return plugin.Activation{}, ErrUnavailable
	}
	return owned.managed.activation()
}

func (c *connections) serviceSession(ctx context.Context, current mcpserver.Server) (*sdk.ClientSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.cancellationErr(ctx); err != nil {
		return nil, err
	}
	configured := c.find(current.ID())
	if c.closed || configured == nil || configured.session == nil || !configured.config.SameConnection(current) {
		return nil, ErrUnavailable
	}
	return configured.session, nil
}

func (c *connections) waitSession(ctx context.Context, current mcpserver.Server) (*sdk.ClientSession, error) {
	for {
		c.mu.Lock()
		if err := c.cancellationErr(ctx); err != nil {
			c.mu.Unlock()
			return nil, err
		}
		if c.closed {
			c.mu.Unlock()
			return nil, ErrUnavailable
		}
		configured := c.find(current.ID())
		if configured == nil {
			c.mu.Unlock()
			return nil, ErrUnavailable
		}
		if configured.session != nil && configured.config.SameConnection(current) {
			session := configured.session
			c.mu.Unlock()
			return session, nil
		}
		attempt := configured.attempt
		c.mu.Unlock()
		if attempt == nil {
			return nil, ErrUnavailable
		}
		select {
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), context.Cause(ctx))
		case <-c.lifetime.Done():
			return nil, errors.Join(c.lifetime.Err(), context.Cause(c.lifetime))
		case <-attempt.done:
			if attempt.err != nil {
				return nil, errors.Join(ErrUnavailable, attempt.err)
			}
		}
	}
}
