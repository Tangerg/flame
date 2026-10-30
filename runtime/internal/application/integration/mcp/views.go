package mcp

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/secrets"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// SecretChangeKind is the application's exact secret-mutation vocabulary.
// Persistence never has to infer intent from an empty string.
type SecretChangeKind string

const (
	SecretSet   SecretChangeKind = "set"
	SecretClear SecretChangeKind = "clear"
)

// SecretChange is a write-only mutation of one credential-bearing value: the
// bearer token, or a full replacement of HTTP headers or a process environment.
type SecretChange[T any] struct {
	Kind  SecretChangeKind
	Value T
}

// ConnectionInput is a complete connection replacement. Transport-specific
// validation happens before it becomes the domain's flat persistence descriptor.
type ConnectionInput struct {
	Transport     mcpserver.Transport
	URL           string
	Authorization *SecretChange[string]
	Headers       *SecretChange[map[string]string]
	Command       string
	Args          []string
	Environment   *SecretChange[map[string]string]
	Dir           string
}

// ServerInput is a complete create/test candidate.
type ServerInput struct {
	Name             mcpserver.ServerName
	Enabled          bool
	Description      string
	Connection       ConnectionInput
	HandshakeTimeout mcpserver.HandshakeTimeout
}

// ServerPatch is an update command. nil preserves the current value; a
// present zero or empty collection clears it.
type ServerPatch struct {
	Enabled          *bool
	Description      *string
	Connection       *ConnectionInput
	HandshakeTimeout *mcpserver.HandshakeTimeout
}

// Empty reports whether the update carries no mutation.
func (s ServerPatch) Empty() bool {
	return s.Enabled == nil && s.Description == nil && s.Connection == nil && s.HandshakeTimeout == nil
}

// Connection is the safe application read model for a connection. Raw
// secret-bearing values never cross the application boundary.
type Connection struct {
	Transport           mcpserver.Transport
	URL                 string
	AuthorizationMasked string
	HeadersMasked       map[string]string
	Command             string
	Args                []string
	EnvironmentMasked   map[string]string
	Dir                 string
}

// Server is the unified application read model: durable configuration and
// the current connection lifecycle are projected together.
type Server struct {
	ID               mcpserver.ID
	Description      string
	Connection       Connection
	HandshakeTimeout mcpserver.HandshakeTimeout
	State            ServerState
}

// ServerState is the application's complete server lifecycle. Disabled is
// represented explicitly rather than as an absent or contradictory status.
// Failure names the category exactly when Type is failed.
type ServerState struct {
	Type      ServerStateType
	ToolCount *int
	Failure   mcpserver.ConnectionFailure
}

type ServerStateType string

const (
	ServerDisabled     ServerStateType = "disabled"
	ServerDisconnected ServerStateType = "disconnected"
	ServerConnecting   ServerStateType = "connecting"
	ServerConnected    ServerStateType = "connected"
	ServerFailed       ServerStateType = "failed"
	ServerNeedsAuth    ServerStateType = "needsAuth"
)

// ServerStatus is the application status notification read model. Known is
// false after a removed server's final invalidation.
type ServerStatus struct {
	Server    mcpserver.ID
	Known     bool
	State     mcpserver.ConnectionState
	Failure   mcpserver.ConnectionFailure
	ToolCount *int
}

// TestResult is the semantic outcome of a non-persisting connection probe.
type TestResult string

const (
	TestSucceeded             TestResult = "succeeded"
	TestAuthorizationRequired TestResult = "authorization_required"
	TestTimedOut              TestResult = "timeout"
	TestFailed                TestResult = "failed"
)

func connectionView(server mcpserver.Server) Connection {
	view := Connection{Transport: server.Transport()}
	switch connection := server.Connection.(type) {
	case mcpserver.HTTPConnection:
		view.URL = connection.URL
		view.AuthorizationMasked = secrets.Mask(connection.Authorization)
		view.HeadersMasked = maskedValues(connection.Headers)
	case mcpserver.StdioConnection:
		view.Command = connection.Command
		view.Args = slices.Clone(connection.Args)
		view.EnvironmentMasked = maskedValues(connection.Env)
		view.Dir = connection.Dir
	}
	return view
}

func maskedValues(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	masked := make(map[string]string, len(values))
	for key, value := range values {
		masked[key] = secrets.Mask(value)
	}
	return masked
}

func serverView(server mcpserver.Server, status *ServerStatus) (Server, error) {
	view := Server{
		ID:               server.ID(),
		Description:      server.Description,
		Connection:       connectionView(server),
		HandshakeTimeout: server.HandshakeTimeout,
		State:            ServerState{Type: ServerDisconnected},
	}
	if !server.Enabled {
		view.State.Type = ServerDisabled
		return view, nil
	}
	if status == nil || !status.Known {
		return view, nil
	}
	switch status.State {
	case mcpserver.ConnectionConnecting:
		view.State.Type = ServerConnecting
	case mcpserver.ConnectionConnected:
		view.State.Type = ServerConnected
		view.State.ToolCount = status.ToolCount
	case mcpserver.ConnectionFailed:
		view.State = ServerState{Type: ServerFailed, Failure: status.Failure}
	case mcpserver.ConnectionNeedsAuth:
		view.State.Type = ServerNeedsAuth
	default:
		return Server{}, fmt.Errorf("mcp: project connection state %q", status.State)
	}
	return view, nil
}

func statusView(status mcpserver.ConnectionStatus) ServerStatus {
	view := ServerStatus{Server: status.Server, Known: true, State: status.State, Failure: status.Failure}
	if status.State == mcpserver.ConnectionConnected {
		count := status.ToolCount
		view.ToolCount = &count
	}
	return view
}
