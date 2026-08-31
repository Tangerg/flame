package delivery

import (
	"fmt"
	"math"
	"time"

	mcpapp "github.com/Tangerg/flame/runtime/internal/application/mcp"
	"github.com/Tangerg/flame/runtime/internal/domain/mcpserver"
	"github.com/Tangerg/flame/runtime/protocol"
)

const maxMCPHandshakeTimeoutSeconds = int64(math.MaxInt64) / int64(time.Second)

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
	policy, err := mcpToolPolicyFromWire(in.DisabledTools, in.AutoApproveTools)
	if err != nil {
		return mcpapp.ServerInput{}, err
	}
	return mcpapp.ServerInput{
		Name:             name,
		Enabled:          in.Enabled,
		Description:      in.Description,
		Connection:       connection,
		HandshakeTimeout: timeout,
		ToolPolicy:       policy,
	}, nil
}

func parseMCPServerName(raw string) (mcpserver.ServerName, error) {
	name, err := mcpserver.ParseServerName(raw)
	if err != nil {
		return mcpserver.ServerName{}, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return name, nil
}

func mcpServerPatchFromRequest(in protocol.UpdateMCPServerRequest) (mcpapp.ServerPatch, error) {
	patch := mcpapp.ServerPatch{
		Enabled:     in.Enabled,
		Description: in.Description,
	}
	if in.DisabledTools != nil {
		disabled, err := parseRemoteToolNames(*in.DisabledTools)
		if err != nil {
			return mcpapp.ServerPatch{}, err
		}
		patch.DisabledTools = &disabled
	}
	if in.AutoApproveTools != nil {
		autoApproved, err := parseRemoteToolNames(*in.AutoApproveTools)
		if err != nil {
			return mcpapp.ServerPatch{}, err
		}
		patch.AutoApproveTools = &autoApproved
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

func mcpToolPolicyFromWire(disabledRaw, autoApprovedRaw []string) (mcpserver.ServerToolPolicy, error) {
	disabled, err := parseRemoteToolNames(disabledRaw)
	if err != nil {
		return mcpserver.ServerToolPolicy{}, err
	}
	autoApproved, err := parseRemoteToolNames(autoApprovedRaw)
	if err != nil {
		return mcpserver.ServerToolPolicy{}, err
	}
	policy, err := mcpserver.NewServerToolPolicy(disabled, autoApproved)
	if err != nil {
		return mcpserver.ServerToolPolicy{}, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return policy, nil
}

func parseRemoteToolNames(raw []string) ([]mcpserver.RemoteToolName, error) {
	names := make([]mcpserver.RemoteToolName, len(raw))
	for i, value := range raw {
		name, err := mcpserver.ParseRemoteToolName(value)
		if err != nil {
			return nil, fmt.Errorf("%w: remote tool at index %d: %w", protocol.ErrInvalidParams, i, err)
		}
		names[i] = name
	}
	return names, nil
}

func mcpHandshakeTimeoutFromWire(in protocol.MCPHandshakeTimeout) (mcpserver.HandshakeTimeout, error) {
	switch in.Type {
	case protocol.MCPHandshakeUnbounded:
		return mcpserver.HandshakeTimeout{}, nil
	case protocol.MCPHandshakeBounded:
		if in.Seconds == nil {
			return mcpserver.HandshakeTimeout{}, fmt.Errorf("%w: bounded MCP handshake timeout requires seconds", protocol.ErrInvalidParams)
		}
		if int64(*in.Seconds) > maxMCPHandshakeTimeoutSeconds {
			return mcpserver.HandshakeTimeout{}, fmt.Errorf("%w: MCP handshake timeout exceeds time.Duration", protocol.ErrInvalidParams)
		}
		timeout, err := mcpserver.NewHandshakeTimeout(time.Duration(*in.Seconds) * time.Second)
		if err != nil {
			return mcpserver.HandshakeTimeout{}, fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
		}
		return timeout, nil
	default:
		return mcpserver.HandshakeTimeout{}, fmt.Errorf("%w: unknown MCP handshake timeout %q", protocol.ErrInvalidParams, in.Type)
	}
}

func mcpConnectionInputFromWire(in protocol.MCPConnectionInput) (mcpapp.ConnectionInput, error) {
	transport, ok := mcpTransportFromWire(in.Type)
	if !ok {
		return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP transport %q", protocol.ErrInvalidParams, in.Type)
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
			return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP authorization change %q", protocol.ErrInvalidParams, in.Authorization.Type)
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
			return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP headers change %q", protocol.ErrInvalidParams, in.Headers.Type)
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
			return mcpapp.ConnectionInput{}, fmt.Errorf("%w: unknown MCP environment change %q", protocol.ErrInvalidParams, in.Env.Type)
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
