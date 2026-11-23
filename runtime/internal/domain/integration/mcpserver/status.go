package mcpserver

import (
	"errors"

	toolcontract "github.com/Tangerg/scope/core/tool"
)

type ConnectionState string

const (
	ConnectionConnecting ConnectionState = "connecting"
	ConnectionConnected  ConnectionState = "connected"
	ConnectionFailed     ConnectionState = "failed"
	ConnectionNeedsAuth  ConnectionState = "needsAuth"
)

type ConnectionFailure string

const (
	FailureConfiguration ConnectionFailure = "configuration"
	FailureConnection    ConnectionFailure = "connection"
	FailureToolDiscovery ConnectionFailure = "toolDiscovery"
	FailureAuthorization ConnectionFailure = "authorization"
)

type ConnectionStatus struct {
	Server    ID
	State     ConnectionState
	Failure   ConnectionFailure
	ToolCount int
}

var ErrAuthorizationRequired = errors.New("mcpserver: authorization required")

type AdvertisedTool struct {
	Server     ID
	Name       RemoteToolName
	Definition toolcontract.Definition
}
