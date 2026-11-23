package mcp

import (
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/integration/secrets"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

type SecretChangeKind string

const (
	SecretSet   SecretChangeKind = "set"
	SecretClear SecretChangeKind = "clear"
)

type SecretChange[T any] struct {
	Kind  SecretChangeKind
	Value T
}

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

type ServerInput struct {
	Name             mcpserver.ServerName
	Enabled          bool
	Description      string
	Connection       ConnectionInput
	HandshakeTimeout mcpserver.HandshakeTimeout
}

type ServerPatch struct {
	Enabled          *bool
	Description      *string
	Connection       *ConnectionInput
	HandshakeTimeout *mcpserver.HandshakeTimeout
}

func (s ServerPatch) Empty() bool {
	return s.Enabled == nil && s.Description == nil && s.Connection == nil && s.HandshakeTimeout == nil
}

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

type Server struct {
	ID               mcpserver.ID
	Description      string
	Connection       Connection
	HandshakeTimeout mcpserver.HandshakeTimeout
	State            ServerState
}

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

func serverView(server mcpserver.Server, status *mcpserver.ConnectionStatus) (Server, error) {
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
	if status == nil {
		return view, nil
	}
	switch status.State {
	case mcpserver.ConnectionConnecting:
		view.State.Type = ServerConnecting
	case mcpserver.ConnectionConnected:
		view.State.Type = ServerConnected
		view.State.ToolCount = &status.ToolCount
	case mcpserver.ConnectionFailed:
		view.State = ServerState{Type: ServerFailed, Failure: status.Failure}
	case mcpserver.ConnectionNeedsAuth:
		view.State.Type = ServerNeedsAuth
	default:
		return Server{}, fmt.Errorf("mcp: project connection state %q", status.State)
	}
	return view, nil
}
