package protocol

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/mcpconnector"
)

//flame:enum
type MCPOriginType string

const (
	MCPOriginUser         MCPOriginType = "user"
	MCPOriginInstallation MCPOriginType = "installation"
)

type MCPOrigin struct {
	Type           MCPOriginType `json:"type"`
	InstallationID string        `json:"installationId,omitempty"`
}

type MCPServerID struct {
	Origin MCPOrigin `json:"origin"`
	Name   string    `json:"name"`
}

type MCPServerRequest struct {
	Server MCPServerID `json:"server"`
}

type CreateMCPAuthorizationAttemptRequest struct {
	Server MCPServerID `json:"server"`
}

type MCPAuthorizationAttemptRequest struct {
	AttemptID string `json:"attemptId"`
}

type MCPListToolsRequest struct {
	Server *MCPServerID `json:"server,omitzero"`
}

type MCPServer struct {
	ID               MCPServerID         `json:"id"`
	Description      string              `json:"description,omitempty"`
	Connection       MCPConnection       `json:"connection"`
	HandshakeTimeout MCPHandshakeTimeout `json:"handshakeTimeout"`
	Status           MCPServerState      `json:"status"`
}

//flame:enum
type MCPHandshakeTimeoutType string

const (
	MCPHandshakeUnbounded MCPHandshakeTimeoutType = "unbounded"
	MCPHandshakeBounded   MCPHandshakeTimeoutType = "bounded"
)

type MCPHandshakeTimeout struct {
	Type    MCPHandshakeTimeoutType `json:"type"`
	Seconds *int                    `json:"seconds,omitzero"`
}

//flame:enum
type MCPServerStateType string

const (
	MCPServerDisabled     MCPServerStateType = "disabled"
	MCPServerDisconnected MCPServerStateType = "disconnected"
	MCPServerConnecting   MCPServerStateType = "connecting"
	MCPServerConnected    MCPServerStateType = "connected"
	MCPServerFailed       MCPServerStateType = "failed"
	MCPServerNeedsAuth    MCPServerStateType = "needsAuth"
)

type MCPServerState struct {
	Type      MCPServerStateType `json:"type"`
	ToolCount *int               `json:"toolCount,omitzero"`
	Error     *MCPStatusProblem  `json:"error,omitzero"`
}

// The category is a localization key; the cause stays in Runtime traces because it
// can carry paths, endpoints or credentials.
type MCPStatusProblem struct {
	Type MCPStatusProblemType `json:"type"`
}

//flame:enum
type MCPStatusProblemType string

const (
	MCPStatusAuthorizationRequired MCPStatusProblemType = "mcp_authorization_required"
	MCPStatusAuthorizationFailed   MCPStatusProblemType = "mcp_authorization_failed"
	MCPStatusDialFailed            MCPStatusProblemType = "mcp_dial_failed"
	MCPStatusToolDiscoveryFailed   MCPStatusProblemType = "mcp_tool_discovery_failed"
	MCPStatusConfigurationFailed   MCPStatusProblemType = "mcp_configuration_failed"
)

//flame:enum
type MCPTransport string

const (
	MCPTransportStdio          MCPTransport = "stdio"
	MCPTransportStreamableHTTP MCPTransport = "streamableHttp"
)

type MCPConnection struct {
	Type                MCPTransport      `json:"type"`
	URL                 string            `json:"url,omitempty"`
	AuthorizationMasked string            `json:"authorizationMasked,omitempty"`
	HeadersMasked       map[string]string `json:"headersMasked,omitempty"`
	Command             string            `json:"command,omitempty"`
	Args                []string          `json:"args,omitempty"`
	EnvMasked           map[string]string `json:"envMasked,omitempty"`
	Dir                 string            `json:"dir,omitempty"`
}

//flame:enum
type MCPSecretChangeType string

const (
	MCPSecretSet   MCPSecretChangeType = "set"
	MCPSecretClear MCPSecretChangeType = "clear"
)

type MCPAuthorizationChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value string              `json:"value,omitempty"`
}

type MCPHeadersChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

type MCPEnvironmentChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

type MCPConnectionInput struct {
	Type          MCPTransport            `json:"type"`
	URL           string                  `json:"url,omitempty"`
	Authorization *MCPAuthorizationChange `json:"authorization,omitzero"`
	Headers       *MCPHeadersChange       `json:"headers,omitzero"`
	Command       string                  `json:"command,omitempty"`
	Args          []string                `json:"args,omitempty"`
	Env           *MCPEnvironmentChange   `json:"env,omitzero"`
	Dir           string                  `json:"dir,omitempty"`
}

type MCPServerCandidate struct {
	Name             string              `json:"name"`
	Enabled          bool                `json:"enabled"`
	Description      string              `json:"description,omitempty"`
	Connection       MCPConnectionInput  `json:"connection"`
	HandshakeTimeout MCPHandshakeTimeout `json:"handshakeTimeout"`
}

type UpdateMCPServerRequest struct {
	Server           MCPServerID          `json:"server"`
	Enabled          *bool                `json:"enabled,omitzero"`
	Description      *string              `json:"description,omitzero"`
	Connection       *MCPConnectionInput  `json:"connection,omitzero"`
	HandshakeTimeout *MCPHandshakeTimeout `json:"handshakeTimeout,omitzero"`
}

type MCPTool struct {
	ModelName     string         `json:"modelName"`
	NameConflicts []ToolRef      `json:"nameConflicts"`
	Server        MCPServerID    `json:"server"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	InputSchema   map[string]any `json:"inputSchema,omitempty"`
}

//flame:enum
type MCPTestOutcome string

const (
	MCPTestReachable             MCPTestOutcome = "reachable"
	MCPTestAuthorizationRequired MCPTestOutcome = "authorizationRequired"
	MCPTestTimedOut              MCPTestOutcome = "timedOut"
	MCPTestFailed                MCPTestOutcome = "failed"
)

type MCPTestResult struct {
	Outcome MCPTestOutcome `json:"outcome"`
}

//flame:enum
type MCPAuthorizationAttemptStatusType string

const (
	MCPAuthorizationAttemptUnconfirmed MCPAuthorizationAttemptStatusType = MCPAuthorizationAttemptStatusType(mcpconnector.Unconfirmed)
	MCPAuthorizationAttemptPending     MCPAuthorizationAttemptStatusType = MCPAuthorizationAttemptStatusType(mcpconnector.Pending)
	MCPAuthorizationAttemptSucceeded   MCPAuthorizationAttemptStatusType = MCPAuthorizationAttemptStatusType(mcpconnector.Succeeded)
	MCPAuthorizationAttemptFailed      MCPAuthorizationAttemptStatusType = MCPAuthorizationAttemptStatusType(mcpconnector.Failed)
	MCPAuthorizationAttemptCanceled    MCPAuthorizationAttemptStatusType = MCPAuthorizationAttemptStatusType(mcpconnector.Canceled)
)

type MCPAuthorizationAttemptStatus struct {
	Type  MCPAuthorizationAttemptStatusType `json:"type"`
	Error *MCPStatusProblem                 `json:"error,omitzero"`
}

type MCPAuthorizationAttempt struct {
	ID         string                        `json:"id"`
	Server     MCPServerID                   `json:"server"`
	Status     MCPAuthorizationAttemptStatus `json:"status"`
	CreatedAt  time.Time                     `json:"createdAt,omitzero"`
	FinishedAt *time.Time                    `json:"finishedAt,omitzero"`
}

type MCPToolExposure struct {
	Server        MCPServerID `json:"server"`
	DisabledTools []string    `json:"disabledTools"`
}
type SetMCPToolExposureRequest struct {
	Server   MCPServerID `json:"server"`
	Name     string      `json:"name"`
	Disabled bool        `json:"disabled"`
}
