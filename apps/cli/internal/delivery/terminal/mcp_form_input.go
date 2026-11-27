package terminal

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type mcpFormDraft struct {
	name              string
	enabled           bool
	description       string
	replaceConnection bool
	transport         protocol.MCPTransport
	url               string
	authorizationMode formChange
	authorization     string
	headersMode       formChange
	headers           string
	command           string
	arguments         string
	environmentMode   formChange
	environment       string
	directory         string
	timeoutSeconds    string
}

func newMCPFormDraft(mode mcpFormMode, server protocol.MCPServer) mcpFormDraft {
	draft := mcpFormDraft{
		enabled: true, replaceConnection: true, transport: protocol.MCPTransportStreamableHTTP,
		authorizationMode: formChangeKeep, headersMode: formChangeKeep, environmentMode: formChangeKeep,
	}
	if mode != mcpFormUpdate {
		return draft
	}
	draft.name = server.ID.Name
	if server.Status.Type == protocol.MCPServerDisabled {
		draft.enabled = false
	}
	draft.description = server.Description
	draft.replaceConnection = false
	draft.transport = server.Connection.Type
	draft.url = server.Connection.URL
	draft.command = server.Connection.Command
	if len(server.Connection.Args) > 0 {
		encoded, _ := json.Marshal(server.Connection.Args)
		draft.arguments = string(encoded)
	}
	draft.directory = server.Connection.Dir
	if server.HandshakeTimeout.Type == protocol.MCPHandshakeBounded {
		draft.timeoutSeconds = strconv.Itoa(*server.HandshakeTimeout.Seconds)
	}
	return draft
}

func (m mcpFormDraft) candidate() (protocol.MCPServerCandidate, error) {
	connection, err := m.connection()
	if err != nil {
		return protocol.MCPServerCandidate{}, err
	}
	timeout, err := parseMCPTimeout(m.timeoutSeconds)
	if err != nil {
		return protocol.MCPServerCandidate{}, err
	}
	candidate := protocol.MCPServerCandidate{
		Name: strings.TrimSpace(m.name), Enabled: m.enabled,
		Description: strings.TrimSpace(m.description), Connection: connection,
		HandshakeTimeout: timeout,
	}
	return candidate, protocol.ValidateWireTree(candidate)
}

func (m mcpFormDraft) update(original protocol.MCPServer) (protocol.UpdateMCPServerRequest, bool, error) {
	timeout, err := parseMCPTimeout(m.timeoutSeconds)
	if err != nil {
		return protocol.UpdateMCPServerRequest{}, false, err
	}
	update := protocol.UpdateMCPServerRequest{
		MCPDirectoryTarget: protocol.MCPDirectoryTarget{Server: original.ID, ExpectedRevision: original.Revision},
	}
	enabled := m.enabled
	if enabled != (original.Status.Type != protocol.MCPServerDisabled) {
		update.Enabled = &enabled
	}
	description := strings.TrimSpace(m.description)
	if description != original.Description {
		update.Description = &description
	}
	previousTimeout := original.HandshakeTimeout
	if timeout.Type != previousTimeout.Type ||
		(timeout.Type == protocol.MCPHandshakeBounded && *timeout.Seconds != *previousTimeout.Seconds) {
		update.HandshakeTimeout = &timeout
	}
	if m.replaceConnection {
		connection, err := m.connection()
		if err != nil {
			return protocol.UpdateMCPServerRequest{}, false, err
		}
		update.Connection = &connection
	}
	changed := update.Enabled != nil || update.Description != nil || update.Connection != nil || update.HandshakeTimeout != nil
	return update, changed, protocol.ValidateWireTree(update)
}

func (m mcpFormDraft) connection() (protocol.MCPConnectionInput, error) {
	connection := protocol.MCPConnectionInput{Type: m.transport}
	switch m.transport {
	case protocol.MCPTransportStreamableHTTP:
		connection.URL = strings.TrimSpace(m.url)
		authorization, err := mcpAuthorizationChange(m.authorizationMode, m.authorization)
		if err != nil {
			return protocol.MCPConnectionInput{}, err
		}
		connection.Authorization = authorization
		headers, err := mcpHeadersChange(m.headersMode, m.headers)
		if err != nil {
			return protocol.MCPConnectionInput{}, err
		}
		connection.Headers = headers
	case protocol.MCPTransportStdio:
		connection.Command = m.command
		arguments, err := parseMCPArguments(m.arguments)
		if err != nil {
			return protocol.MCPConnectionInput{}, err
		}
		connection.Args = arguments
		connection.Dir = m.directory
		environment, err := mcpEnvironmentChange(m.environmentMode, m.environment)
		if err != nil {
			return protocol.MCPConnectionInput{}, err
		}
		connection.Env = environment
	}
	return connection, nil
}

func mcpAuthorizationChange(mode formChange, value string) (*protocol.MCPAuthorizationChange, error) {
	switch mode {
	case formChangeSet:
		return &protocol.MCPAuthorizationChange{Type: protocol.MCPSecretSet, Value: value}, nil
	case formChangeClear:
		return &protocol.MCPAuthorizationChange{Type: protocol.MCPSecretClear}, nil
	case formChangeKeep:
		return nil, nil
	default:
		return nil, mode.Validate()
	}
}

func mcpHeadersChange(mode formChange, value string) (*protocol.MCPHeadersChange, error) {
	switch mode {
	case formChangeKeep:
		return nil, nil
	case formChangeClear:
		return &protocol.MCPHeadersChange{Type: protocol.MCPSecretClear}, nil
	case formChangeSet:
		values, err := parseMCPStringMap(value)
		if err != nil {
			return nil, err
		}
		change := &protocol.MCPHeadersChange{Type: protocol.MCPSecretSet, Value: values}
		return change, protocol.ValidateWireTree(change)
	default:
		return nil, mode.Validate()
	}
}

func mcpEnvironmentChange(mode formChange, value string) (*protocol.MCPEnvironmentChange, error) {
	switch mode {
	case formChangeKeep:
		return nil, nil
	case formChangeClear:
		return &protocol.MCPEnvironmentChange{Type: protocol.MCPSecretClear}, nil
	case formChangeSet:
		values, err := parseMCPStringMap(value)
		if err != nil {
			return nil, err
		}
		change := &protocol.MCPEnvironmentChange{Type: protocol.MCPSecretSet, Value: values}
		return change, protocol.ValidateWireTree(change)
	default:
		return nil, mode.Validate()
	}
}

// JSON null otherwise decodes as an empty string, which is valid process material.
var mcpFormStringDecoding = json.WithUnmarshalers(json.UnmarshalFromFunc(func(decoder *jsontext.Decoder, value *string) error {
	if decoder.PeekKind() != jsontext.KindString {
		return errors.New("must be a JSON string")
	}
	token, err := decoder.ReadToken()
	if err != nil {
		return err
	}
	*value = token.String()
	return nil
}))

func parseMCPStringMap(value string) (map[string]string, error) {
	var parsed map[string]string
	if err := json.Unmarshal([]byte(value), &parsed, mcpFormStringDecoding); err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, errors.New("expected a JSON object")
	}
	return parsed, nil
}

func parseMCPArguments(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	var arguments []string
	if err := json.Unmarshal([]byte(value), &arguments, mcpFormStringDecoding); err != nil {
		return nil, err
	}
	if arguments == nil {
		return nil, errors.New("expected a JSON argument array")
	}
	return arguments, nil
}

func parseMCPTimeout(value string) (protocol.MCPHandshakeTimeout, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded}, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil {
		return protocol.MCPHandshakeTimeout{}, errors.New("handshake timeout must be a positive integer")
	}
	timeout := protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeBounded, Seconds: &seconds}
	return timeout, protocol.ValidateWireTree(timeout)
}
