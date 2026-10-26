package mcp

import (
	"context"
	"crypto/sha256"
	json "encoding/json/v2"
	"errors"
	"os/exec"
	"slices"
	"strings"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/cancellation"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
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

	// oauth is either a restored durable OAuth handler or the handler obtained by
	// a successful [connections.Authorize]. nil until a saved session exists or
	// the user signs in. It is reusable only for the same credential configuration.
	oauth auth.OAuthHandler

	attempt *attempt
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
	closeFn  func() error
	close    *sessionCloseAttempt
	bindings *plugin.RequiredBindings
}

// connections owns the live MCP server sessions + reconnect. The tool sink
// receives the rebuilt model-facing tool set inside the same critical section
// that changes a server's live outcome, so no reader can observe a settled
// status whose tools the sink has not received, or the reverse.
type connections struct {
	lifetime  context.Context
	callbacks *pluginsdk.Callbacks
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
	attempts sync.WaitGroup
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

func (c *connections) ownSessionLocked(session *sdkmcp.ClientSession, cleanup sessionCleanup, bindings *plugin.RequiredBindings) {
	if session == nil {
		return
	}
	if c.sessions == nil {
		c.sessions = make(map[*sdkmcp.ClientSession]*ownedSession)
	}
	if c.sessions[session] == nil {
		if c.callbacks != nil {
			c.callbacks.AdmitSession(session)
		}
		c.sessions[session] = &ownedSession{bindings: bindings, closeFn: func() error {
			if c.callbacks != nil {
				c.callbacks.RevokeSession(session)
			}
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
	if c.closed || session == nil {
		return plugin.Activation{}, ErrUnavailable
	}
	var root *server
	for _, candidate := range c.servers {
		if candidate.session == session {
			root = candidate
			break
		}
	}
	type boundJSON struct{ Provider, Digest, Contribution, Server, Export, Contract string }
	type requirementJSON struct {
		Contribution, ID, Contract string
		Optional                   bool
		Bindings                   []boundJSON
	}
	type entryJSON struct {
		Server, Authority string
		Requirements      []requirementJSON
	}
	var entries []entryJSON
	var dependencies []plugin.Dependency
	visited := map[*server]bool{}
	var collect func(*server) error
	collect = func(current *server) error {
		if current == nil || current.session == nil {
			return ErrUnavailable
		}
		if visited[current] {
			return nil
		}
		visited[current] = true
		owned := c.sessions[current.session]
		if owned == nil || owned.close != nil {
			return ErrUnavailable
		}
		if digest, _, installed := current.config.Source.Release(); installed {
			id, _ := current.id.Origin().Installation()
			dependencies = append(dependencies, plugin.Dependency{InstallationID: id, Digest: digest})
		}
		entry := entryJSON{Server: current.id.String(), Authority: current.config.AuthorityFingerprint().String(), Requirements: []requirementJSON{}}
		for _, required := range owned.bindings.Snapshot() {
			requirement := requirementJSON{Contribution: required.Contribution.String(), ID: required.ID.String(), Contract: required.Contract.String(), Optional: required.Optional, Bindings: []boundJSON{}}

			for _, binding := range required.Bindings {
				requirement.Bindings = append(requirement.Bindings, boundJSON{binding.Provider.InstallationID.String(), binding.Provider.Digest.String(), binding.Contribution.String(), binding.Server.String(), binding.Export, binding.Contract.String()})
				origin, err := mcpserver.InstallationOrigin(binding.Provider.InstallationID)
				if err != nil {
					return err
				}
				id, err := mcpserver.NewID(origin, binding.Server)
				if err != nil {
					return err
				}
				provider := c.find(id)
				if provider == nil {
					return ErrUnavailable
				}
				digest, _, installed := provider.config.Source.Release()
				if !installed || digest != binding.Provider.Digest {
					return ErrUnavailable
				}
				if err := collect(provider); err != nil {
					return err
				}
			}
			entry.Requirements = append(entry.Requirements, requirement)
		}
		entries = append(entries, entry)
		return nil
	}
	if err := collect(root); err != nil {
		return plugin.Activation{}, err
	}
	slices.SortFunc(entries, func(a, b entryJSON) int { return strings.Compare(a.Server, b.Server) })
	encoded, err := json.Marshal(entries, json.Deterministic(true))
	if err != nil {
		return plugin.Activation{}, err
	}
	return plugin.Activation{Identity: identity.DigestFromSum(sha256.Sum256(encoded)), Dependencies: plugin.CompactDependencies(dependencies)}, nil
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
