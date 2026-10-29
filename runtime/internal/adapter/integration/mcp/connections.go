package mcp

import (
	"context"
	"errors"
	"os/exec"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/cancellation"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/go-sdk/auth"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"
)

// server is the live state of one configured MCP server. Access is guarded by
// connections.mu; an exact attempt registration gives each server
// latest-operation-wins semantics without serializing unrelated servers or
// waiting OAuth flows.
type server struct {
	id      mcpserver.ID
	config  ServerConfig          // zero while the source has refused every connection
	session *sdkmcp.ClientSession // nil when not connected
	tools   []Executable          // last tool set proved on this session
	state   mcpserver.ConnectionState
	failure mcpserver.ConnectionFailure // set exactly while state is failed
	cause   error

	// oauth is either a restored durable OAuth handler or the handler obtained by
	// a successful [connections.Authorize]. nil until a saved session exists or
	// the user signs in. It is reusable only for the same credential configuration.
	oauth auth.OAuthHandler

	attempt *attempt
}

func (s *server) name() mcpserver.ID { return s.id }

// settle is the only writer of the live outcome, so the session, tool set,
// state and failure category always describe the same attempt.
func (s *server) settle(session *sdkmcp.ClientSession, tools []Executable, state mcpserver.ConnectionState, failure mcpserver.ConnectionFailure, cause error) {
	if state != mcpserver.ConnectionConnected {
		session, tools = nil, nil
	}
	if state != mcpserver.ConnectionFailed {
		failure = ""
		cause = nil
	}
	s.session, s.tools, s.state, s.failure, s.cause = session, tools, state, failure, cause
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
	closeFn  func() error
	managed  *installedObservation
	close    *sessionCloseAttempt
	bindings *plugin.RequiredBindings
	end      *sessionEnd
}

type sessionEnd struct {
	done chan struct{}
	err  error
}

// connections owns ordinary MCP sessions and observes managed Host instances.
// The tool sink
// receives the rebuilt model-facing tool set inside the same critical section
// that changes a server's live outcome, so no reader can observe a settled
// status whose tools the sink has not received, or the reverse.
type connections struct {
	lifetime  context.Context
	callbacks *qimumcp.Callbacks
	mu        sync.Mutex
	servers   []*server
	client    *sdkmcp.Client
	// onTools is the tool sink; nil until SetToolSink; guarded by mu. It runs
	// while mu is held, so it must be an in-memory publication that never
	// blocks or calls back into connections.
	onTools       func([]Executable, []mcpserver.ID, []mcpserver.ID)
	closed        bool // terminal admission state set by Shutdown
	shutdownState *shutdownAttempt
	sessions      map[*sdkmcp.ClientSession]*ownedSession
	// retirements retain asynchronous close attempts and their diagnostics until
	// Shutdown joins them. Resource ownership remains exclusively in sessions.
	retirements map[*sessionCloseAttempt]struct{}

	// oauthSessions is the durable credential boundary. It is optional so the
	// infrastructure remains usable in processes that deliberately opt out of
	// persistence; callers that enable OAuth supply it.
	oauthSessions oauthSessionStore

	// attempts joins every in-flight dial/OAuth operation during Shutdown. Add is
	// performed under mu before closed can be set, so no Add races the Wait.
	attempts  sync.WaitGroup
	observers sync.WaitGroup
}

func (c *connections) cancellationErr(ctx context.Context) error {
	return errors.Join(cancellation.Err(ctx), cancellation.Err(c.lifetime))
}

// setToolSink publishes the current catalog and every subsequent connection
// change through the same critical section. The sink must publish in memory
// without blocking or calling back into connections.
func (c *connections) setToolSink(sink func([]Executable, []mcpserver.ID, []mcpserver.ID)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onTools = sink
	c.publishToolsLocked()
}

// newClient builds the shared MCP client identity used for every server's
// session (and re-dials). No per-server handlers are needed, so one suffices.
func newClient() *sdkmcp.Client {
	return sdkmcp.NewClient(&sdkmcp.Implementation{Name: identity.ProductName, Version: "v0.1.0"}, nil)
}

// find returns the server with the given name, or nil. Caller holds mu.
func (c *connections) find(name mcpserver.ID) *server {
	for _, configuredServer := range c.servers {
		if configuredServer.name() == name {
			return configuredServer
		}
	}
	return nil
}

// current reports whether session is still the live connection of name.
func (c *connections) current(name mcpserver.ID, session *sdkmcp.ClientSession) bool {
	if session == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	configuredServer := c.find(name)
	return configuredServer != nil && configuredServer.session == session
}

func (c *connections) ownSessionLocked(session *sdkmcp.ClientSession, cleanup sessionCleanup, bindings *plugin.RequiredBindings) error {
	if session == nil {
		return nil
	}
	var err error
	if c.sessions[session] == nil {
		if c.callbacks != nil {
			err = c.callbacks.AdmitSession(session)
		}
		owned := &ownedSession{bindings: bindings, end: &sessionEnd{done: make(chan struct{})}, closeFn: func() error {
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

// retireSession closes session and then its process group. A stdio server
// that reports an exit status has exited, which is what retirement must
// achieve; a nonzero status after its input closed or it was signaled is how
// many servers stop. Only a session that could not be stopped, or a process
// group cleanup that failed, is a retirement failure.
func retireSession(session *sdkmcp.ClientSession, cleanup sessionCleanup) (err error) {
	defer func() { err = errors.Join(err, cleanup()) }()
	err = session.Close()
	// A canceled command that exits cleanly makes exec.Cmd.Wait return
	// context.Canceled. Cleanup still verifies process-group termination.
	if err == context.Canceled {
		err = nil
	}
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		err = nil
	}
	return err
}

var ErrUnavailable = errors.New("mcp: exact backend connection is unavailable")

func (c *connections) requirements(session *sdkmcp.ClientSession) (*plugin.RequiredBindings, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	owned := c.sessions[session]
	if c.closed || owned == nil || owned.close != nil {
		return nil, ErrUnavailable
	}
	for _, server := range c.servers {
		if server.session == session {
			return owned.bindings, nil
		}
	}
	return nil, ErrUnavailable
}

// activation projects the exact live activation graph, including optional
// collaborators that were bound. Re-reading current selections would retarget a
// waiting execution after a collaborator changed its own selection.
func (c *connections) activation(session *sdkmcp.ClientSession) (plugin.Activation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	owned := c.sessions[session]
	if c.closed || owned == nil || owned.close != nil || owned.managed == nil {
		return plugin.Activation{}, ErrUnavailable
	}
	return owned.managed.activation()
}

func (c *connections) serviceSession(ctx context.Context, current mcpserver.Server) (*sdkmcp.ClientSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.cancellationErr(ctx); err != nil {
		return nil, err
	}
	configured := c.find(current.ID())
	if c.closed || configured == nil || configured.session == nil || !configured.config.Server.SameConnection(current) {
		return nil, ErrUnavailable
	}
	return configured.session, nil
}

// WaitSession joins the already owned attempt; it never starts or retries one.
func (c *connections) waitSession(ctx context.Context, current mcpserver.Server) (*sdkmcp.ClientSession, error) {
	for {
		c.mu.Lock()
		if err := c.cancellationErr(ctx); c.closed || err != nil {
			c.mu.Unlock()
			return nil, errors.Join(ErrUnavailable, err)
		}
		configured := c.find(current.ID())
		if configured == nil {
			c.mu.Unlock()
			return nil, ErrUnavailable
		}
		if configured.session != nil && configured.config.Server.SameConnection(current) {
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
