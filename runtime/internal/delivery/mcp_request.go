package delivery

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

func mcpServerInputFromCandidate(in protocol.MCPServerCandidate) (mcpapp.ServerInput, error) {
	name, err := parseMCPServerName(in.Name)
	if err != nil {
		return mcpapp.ServerInput{}, err
	}
	connection, err := mcpConnectionInputFromWire(in.Connection)
	if err != nil {
		return mcpapp.ServerInput{}, err
	}
	timeout, err := mcpHandshakeTimeoutFromWire(in.HandshakeTimeout)
	if err != nil {
		return mcpapp.ServerInput{}, err
	}
	return mcpapp.ServerInput{
		Name:             name,
		Enabled:          in.Enabled,
		Description:      in.Description,
		Connection:       connection,
		HandshakeTimeout: timeout,
	}, nil
}

func parseMCPServerName(raw string) (mcpserver.ServerName, error) {
	name, err := mcpserver.ParseServerName(raw)
	if err != nil {
		return mcpserver.ServerName{}, protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	}
	return name, nil
}

// mcpServerIDFromWire is the only translation of a wire server identity.
func mcpServerIDFromWire(in protocol.MCPServerID) (mcpserver.ID, error) {
	var origin mcpserver.Origin
	switch in.Origin.Type {
	case protocol.MCPOriginUser:
		if in.Origin.InstallationID != "" {
			return mcpserver.ID{}, protocol.NewFailure(protocol.ErrInvalidParams, "user MCP origin carries no installation")
		}
		origin = mcpserver.UserOrigin()
	case protocol.MCPOriginInstallation:
		installation, err := identity.ParseInstallation(in.Origin.InstallationID)
		if err != nil {
			return mcpserver.ID{}, protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
		}
		if origin, err = mcpserver.InstallationOrigin(installation); err != nil {
			return mcpserver.ID{}, protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
		}
	default:
		return mcpserver.ID{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP origin %q", in.Origin.Type))
	}
	name, err := parseMCPServerName(in.Name)
	if err != nil {
		return mcpserver.ID{}, err
	}
	id, err := mcpserver.NewID(origin, name)
	if err != nil {
		return mcpserver.ID{}, protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	}
	return id, nil
}

func mcpServerPatchFromRequest(in protocol.UpdateMCPServerRequest) (mcpapp.ServerPatch, error) {
	patch := mcpapp.ServerPatch{
		Enabled:     in.Enabled,
		Description: in.Description,
	}
	if in.Connection != nil {
		connection, err := mcpConnectionInputFromWire(*in.Connection)
		if err != nil {
			return mcpapp.ServerPatch{}, err
		}
		patch.Connection = &connection
	}
	if in.HandshakeTimeout != nil {
		timeout, err := mcpHandshakeTimeoutFromWire(*in.HandshakeTimeout)
		if err != nil {
			return mcpapp.ServerPatch{}, err
		}
		patch.HandshakeTimeout = &timeout
	}
	return patch, nil
}

func mcpHandshakeTimeoutFromWire(in protocol.MCPHandshakeTimeout) (mcpserver.HandshakeTimeout, error) {
	switch in.Type {
	case protocol.MCPHandshakeUnbounded:
		return mcpserver.HandshakeTimeout{}, nil
	case protocol.MCPHandshakeBounded:
		if in.Seconds == nil {
			return mcpserver.HandshakeTimeout{}, protocol.NewFailure(protocol.ErrInvalidParams, "bounded MCP handshake timeout requires seconds")
		}
		if int64(*in.Seconds) > protocol.MaximumDurationSeconds {
			return mcpserver.HandshakeTimeout{}, protocol.NewFailure(protocol.ErrInvalidParams, "MCP handshake timeout exceeds time.Duration")
		}
		timeout, err := mcpserver.NewHandshakeTimeout(time.Duration(*in.Seconds) * time.Second)
		if err != nil {
			return mcpserver.HandshakeTimeout{}, protocol.NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
		}
		return timeout, nil
	default:
		return mcpserver.HandshakeTimeout{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP handshake timeout %q", in.Type))
	}
}

func mcpConnectionInputFromWire(in protocol.MCPConnectionInput) (mcpapp.ConnectionInput, error) {
	transport, ok := mcpTransportFromWire(in.Type)
	if !ok {
		return mcpapp.ConnectionInput{}, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP transport %q", in.Type))
	}
	var input mcpapp.ConnectionInput
	var err error
	if in.Authorization != nil {
		if input.Authorization, err = secretChangeFromWire("authorization", in.Authorization.Type, in.Authorization.Value); err != nil {
			return mcpapp.ConnectionInput{}, err
		}
	}
	if in.Headers != nil {
		if input.Headers, err = secretChangeFromWire("headers", in.Headers.Type, in.Headers.Value); err != nil {
			return mcpapp.ConnectionInput{}, err
		}
	}
	if in.Env != nil {
		if input.Environment, err = secretChangeFromWire("environment", in.Env.Type, in.Env.Value); err != nil {
			return mcpapp.ConnectionInput{}, err
		}
	}
	input.Transport, input.URL, input.Command, input.Args, input.Dir = transport, in.URL, in.Command, in.Args, in.Dir
	return input, nil
}

func secretChangeFromWire[T any](name string, kind protocol.MCPSecretChangeType, value T) (*mcpapp.SecretChange[T], error) {
	change := mcpapp.SecretChange[T]{Value: value}
	switch kind {
	case protocol.MCPSecretSet:
		change.Kind = mcpapp.SecretSet
	case protocol.MCPSecretClear:
		change.Kind = mcpapp.SecretClear
	default:
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP %s change %q", name, kind))
	}
	return &change, nil
}

func mcpTransportFromWire(transport protocol.MCPTransport) (mcpserver.Transport, bool) {
	switch transport {
	case protocol.MCPTransportStdio:
		return mcpserver.TransportStdio, true
	case protocol.MCPTransportStreamableHTTP:
		return mcpserver.TransportStreamableHTTP, true
	default:
		return "", false
	}
}
