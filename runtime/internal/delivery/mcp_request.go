package delivery

import (
	"errors"
	"fmt"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/protocol"
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
		return mcpserver.ServerName{}, NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
	}
	return name, nil
}

// mcpServerIDFromWire is the only translation of a wire server identity.
func mcpServerIDFromWire(in protocol.MCPServerID) (mcpserver.ID, error) {
	var origin mcpserver.Origin
	switch in.Origin.Type {
	case protocol.MCPOriginUser:
		if in.Origin.InstallationID != "" {
			return mcpserver.ID{}, NewFailure(protocol.ErrInvalidParams, "user MCP origin carries no installation")
		}
		origin = mcpserver.UserOrigin()
	case protocol.MCPOriginInstallation:
		installation, err := resourceid.ParseInstallation(in.Origin.InstallationID)
		if err != nil {
			return mcpserver.ID{}, NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
		}
		if origin, err = mcpserver.InstallationOrigin(installation); err != nil {
			return mcpserver.ID{}, NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
		}
	default:
		return mcpserver.ID{}, NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP origin %q", in.Origin.Type))
	}
	name, err := parseMCPServerName(in.Name)
	if err != nil {
		return mcpserver.ID{}, err
	}
	id, err := mcpserver.NewID(origin, name)
	if err != nil {
		return mcpserver.ID{}, NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
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
			return mcpserver.HandshakeTimeout{}, NewFailure(protocol.ErrInvalidParams, "bounded MCP handshake timeout requires seconds")
		}
		if int64(*in.Seconds) > protocol.MaximumDurationSeconds {
			return mcpserver.HandshakeTimeout{}, NewFailure(protocol.ErrInvalidParams, "MCP handshake timeout exceeds time.Duration")
		}
		timeout, err := mcpserver.NewHandshakeTimeout(time.Duration(*in.Seconds) * time.Second)
		if err != nil {
			return mcpserver.HandshakeTimeout{}, NewFailure(errors.Join(protocol.ErrInvalidParams, err), err.Error())
		}
		return timeout, nil
	default:
		return mcpserver.HandshakeTimeout{}, NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP handshake timeout %q", in.Type))
	}
}

func mcpConnectionInputFromWire(in protocol.MCPConnectionInput) (mcpapp.ConnectionInput, error) {
	transport, ok := mcpTransportFromWire(in.Type)
	if !ok {
		return mcpapp.ConnectionInput{}, NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP transport %q", in.Type))
	}
	var authorization *mcpapp.AuthorizationChange
	if in.Authorization != nil {
		change := mcpapp.AuthorizationChange{Value: in.Authorization.Value}
		switch in.Authorization.Type {
		case protocol.MCPSecretSet:
			change.Kind = mcpapp.SecretSet
		case protocol.MCPSecretClear:
			change.Kind = mcpapp.SecretClear
		default:
			return mcpapp.ConnectionInput{}, NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP authorization change %q", in.Authorization.Type))
		}
		authorization = &change
	}
	var headers *mcpapp.HeadersChange
	if in.Headers != nil {
		change := mcpapp.HeadersChange{Value: in.Headers.Value}
		switch in.Headers.Type {
		case protocol.MCPSecretSet:
			change.Kind = mcpapp.SecretSet
		case protocol.MCPSecretClear:
			change.Kind = mcpapp.SecretClear
		default:
			return mcpapp.ConnectionInput{}, NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP headers change %q", in.Headers.Type))
		}
		headers = &change
	}
	var environment *mcpapp.EnvironmentChange
	if in.Env != nil {
		change := mcpapp.EnvironmentChange{Value: in.Env.Value}
		switch in.Env.Type {
		case protocol.MCPSecretSet:
			change.Kind = mcpapp.SecretSet
		case protocol.MCPSecretClear:
			change.Kind = mcpapp.SecretClear
		default:
			return mcpapp.ConnectionInput{}, NewFailure(protocol.ErrInvalidParams, fmt.Sprintf("unknown MCP environment change %q", in.Env.Type))
		}
		environment = &change
	}
	return mcpapp.ConnectionInput{
		Transport:     transport,
		URL:           in.URL,
		Authorization: authorization,
		Headers:       headers,
		Command:       in.Command,
		Args:          in.Args,
		Environment:   environment,
		Dir:           in.Dir,
	}, nil
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
