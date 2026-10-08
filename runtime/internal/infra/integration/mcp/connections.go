package mcp

import (
	"context"
	"errors"
	"os/exec"
	"sync"

	"github.com/Tangerg/go-sdk/auth"
	sdkmcp "github.com/Tangerg/go-sdk/mcp"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// server is the live state of one configured MCP server. Access is guarded by
// Connections.mu; an exact attempt registration gives each server
// latest-operation-wins semantics without serializing unrelated servers or
// waiting OAuth flows.
type server struct {
	id      mcpserver.ID
	config  ServerConfig          // zero while the source has refused every connection
	session *sdkmcp.ClientSession // nil when not connected
	tools   []Executable          // last tool set proved on this session
	state   mcpserver.ConnectionState
	failure mcpserver.ConnectionFailure // set exactly while state is failed

	// oauth is either a restored durable OAuth handler or the handler obtained by
	// a successful [Connections.Authorize]. nil until a saved session exists or
	// the user signs in. It is reusable only for the same credential configuration.
	oauth auth.OAuthHandler

	attempt *connectionAttempt
}

func (s *server) name() mcpserver.ID { return s.id }

// settle is the only writer of the live outcome, so the session, tool set,
// state and failure category always describe the same attempt.
func (s *server) settle(session *sdkmcp.ClientSession, tools []Executable, state mcpserver.ConnectionState, failure mcpserver.ConnectionFailure) {
	if state != mcpserver.ConnectionConnected {
		session, tools = nil, nil
	}
	if state != mcpserver.ConnectionFailed {
		failure = ""
	}
	s.session, s.tools, s.state, s.failure = session, tools, state, failure
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
	closeFn func() error
	close   *sessionCloseAttempt
}

// Connections owns the live MCP server sessions + reconnect. The tool sink
// receives the rebuilt model-facing tool set inside the same critical section
// that changes a server's live outcome, so no reader can observe a settled
// status whose tools the sink has not received, or the reverse.
type Connections struct {
	lifetime context.Context
	mu       sync.Mutex
	servers  []*server
	client   *sdkmcp.Client
	// onTools is the tool sink; nil until SetToolSink; guarded by mu. It runs
	// while mu is held, so it must be an in-memory publication that never
	// blocks or calls back into Connections.
	onTools  func([]Executable)
	closed   bool // terminal admission state set by Shutdown
	shutdown *shutdownAttempt
	sessions map[*sdkmcp.ClientSession]*ownedSession
	// retirements retain asynchronous close attempts and their diagnostics until
	// Shutdown joins them. Resource ownership remains exclusively in sessions.
	retirements map[*sessionCloseAttempt]struct{}

	// oauthSessions is the durable credential boundary. It is optional so the
	// infrastructure remains usable in processes that deliberately opt out of
	// persistence; callers that enable OAuth supply it.
	oauthSessions OAuthSessionStore

	// attempts joins every in-flight dial/OAuth operation during Shutdown. Add is
	// performed under mu before closed can be set, so no Add races the Wait.
	attempts sync.WaitGroup
}

// SetToolSink publishes the current catalog and every subsequent connection
// change through the same critical section. The sink must publish in memory
// without blocking or calling back into Connections.
func (c *Connections) SetToolSink(sink func([]Executable)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onTools = sink
	c.publishToolsLocked()
}

// newClient builds the shared MCP client identity used for every server's
// session (and re-dials). No per-server handlers are needed, so one suffices.
func newClient() *sdkmcp.Client {
	return sdkmcp.NewClient(&sdkmcp.Implementation{Name: runtimeidentity.ProductName, Version: "v0.1.0"}, nil)
}

// find returns the server with the given name, or nil. Caller holds mu.
func (c *Connections) find(name mcpserver.ID) *server {
	for _, configuredServer := range c.servers {
		if configuredServer.name() == name {
			return configuredServer
		}
	}
	return nil
}

// current reports whether session is still the live connection of name.
func (c *Connections) current(name mcpserver.ID, session *sdkmcp.ClientSession) bool {
	if session == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	configuredServer := c.find(name)
	return configuredServer != nil && configuredServer.session == session
}

func (c *Connections) ownSessionLocked(session *sdkmcp.ClientSession, cleanup sessionCleanup) {
	if session == nil {
		return
	}
	if c.sessions == nil {
		c.sessions = make(map[*sdkmcp.ClientSession]*ownedSession)
	}
	if c.sessions[session] == nil {
		c.sessions[session] = &ownedSession{closeFn: func() error {
			return retireSession(session, cleanup)
		}}
	}
}

// retireSession closes session and then its process group. A stdio server
// that reports an exit status has exited, which is what retirement must
// achieve; a nonzero status after its input closed or it was signaled is how
// many servers stop. Only a session that could not be stopped, or a process
// group cleanup that failed, is a retirement failure.
func retireSession(session *sdkmcp.ClientSession, cleanup sessionCleanup) (err error) {
	defer func() { err = errors.Join(err, cleanup()) }()
	err = session.Close()
	if _, exited := errors.AsType[*exec.ExitError](err); exited {
		err = nil
	}
	return err
}
