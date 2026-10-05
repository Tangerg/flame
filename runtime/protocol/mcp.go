package protocol

import (
	"time"
)

// MCPOriginType names the owner of an MCP server record.
type MCPOriginType string

const (
	MCPOriginUser         MCPOriginType = "user"
	MCPOriginInstallation MCPOriginType = "installation"
)

// MCPOrigin is a closed union naming the only owner that may change a server
// record. Only installation carries installationId.
type MCPOrigin struct {
	Type           MCPOriginType `json:"type"`
	InstallationID string        `json:"installationId,omitempty"`
}

// MCPServerID is the one representation of an MCP server's identity on the
// wire: its owner and the name that owner chose. Names are unique only within
// an origin, so a name is never an identity on its own.
type MCPServerID struct {
	Origin MCPOrigin `json:"origin"`
	Name   string    `json:"name"`
}

// MCPServerRequest identifies one configured MCP server.
type MCPServerRequest struct {
	Server MCPServerID `json:"server"`
}

// CreateMCPAuthorizationAttemptRequest starts one interactive OAuth flow for a
// configured server.
type CreateMCPAuthorizationAttemptRequest struct {
	Server MCPServerID `json:"server"`
}

// MCPAuthorizationAttemptRequest identifies one interactive OAuth flow.
type MCPAuthorizationAttemptRequest struct {
	AttemptID string `json:"attemptId"`
}

// MCPListToolsRequest — mcp.tools.list body. An absent server lists every
// connected server's tools.
type MCPListToolsRequest struct {
	Server *MCPServerID `json:"server,omitzero"`
}

// MCPServer is the single safe read model for one configured MCP server. Server
// list results list user servers first, then installation servers by
// installation, each ordered by name. Status includes "disabled", so
// configuration enablement and live lifecycle can never contradict one another
// on the wire.
type MCPServer struct {
	ID               MCPServerID         `json:"id"`
	Description      string              `json:"description,omitempty"`
	Connection       MCPConnection       `json:"connection"`
	HandshakeTimeout MCPHandshakeTimeout `json:"handshakeTimeout"`
	Status           MCPServerState      `json:"status"`
}

// MCPHandshakeTimeoutType names the complete MCP handshake deadline policy.
// Unbounded means only the operation context can cancel connection setup;
// bounded adds the positive duration carried by seconds.
type MCPHandshakeTimeoutType string

const (
	MCPHandshakeUnbounded MCPHandshakeTimeoutType = "unbounded"
	MCPHandshakeBounded   MCPHandshakeTimeoutType = "bounded"
)

// MCPHandshakeTimeout is a closed union. Only bounded carries seconds, and
// that value is strictly positive.
type MCPHandshakeTimeout struct {
	Type    MCPHandshakeTimeoutType `json:"type"`
	Seconds *int                    `json:"seconds,omitzero"`
}

// MCPServerStateType is the complete lifecycle of a configured MCP server. A
// disabled entry is durable but intentionally has no live connection.
type MCPServerStateType string

const (
	MCPServerDisabled     MCPServerStateType = "disabled"
	MCPServerDisconnected MCPServerStateType = "disconnected"
	MCPServerConnecting   MCPServerStateType = "connecting"
	MCPServerConnected    MCPServerStateType = "connected"
	MCPServerFailed       MCPServerStateType = "failed"
	MCPServerNeedsAuth    MCPServerStateType = "needsAuth"
)

// MCPServerState is a closed union. toolCount belongs only to connected;
// error belongs only to failed, which carries a connection failure category,
// and needsAuth, which carries mcp_authorization_required.
type MCPServerState struct {
	Type      MCPServerStateType `json:"type"`
	ToolCount *int               `json:"toolCount,omitzero"`
	Error     *MCPStatusProblem  `json:"error,omitzero"`
}

// MCPStatusProblem is the inline problem an MCP status carries: its closed
// category and nothing else. The category is a localization key, not
// server-authored copy; the cause stays in Runtime traces because it can
// carry paths, endpoints or credentials. Each status narrows which categories
// it may carry.
type MCPStatusProblem struct {
	Type MCPStatusProblemType `json:"type"`
}

// MCPStatusProblemType is the inline-status problem vocabulary.
type MCPStatusProblemType string

const (
	MCPStatusAuthorizationRequired MCPStatusProblemType = "mcp_authorization_required" // the server requires valid authorization
	MCPStatusAuthorizationFailed   MCPStatusProblemType = "mcp_authorization_failed"   // an interactive sign-in did not complete successfully
	MCPStatusDialFailed            MCPStatusProblemType = "mcp_dial_failed"            // the transport, process or handshake did not succeed
	MCPStatusToolDiscoveryFailed   MCPStatusProblemType = "mcp_tool_discovery_failed"  // a session connected but did not produce a valid tool catalog
	MCPStatusConfigurationFailed   MCPStatusProblemType = "mcp_configuration_failed"   // the configuration or stored credentials could not be used
	MCPStatusReleaseUnavailable    MCPStatusProblemType = "mcp_release_unavailable"    // the owning plugin release cannot be verified
	MCPStatusBackendUnavailable    MCPStatusProblemType = "mcp_backend_unavailable"    // the release is intact but this server's backend cannot be realized
)

// MCPTransport is the protocol's closed MCP transport vocabulary.
type MCPTransport string

const (
	MCPTransportStdio          MCPTransport = "stdio"
	MCPTransportStreamableHTTP MCPTransport = "streamableHttp"
)

// MCPConnection is the safe output union for a server's connection descriptor.
// Secret-bearing values are write-only; reads expose only masked representations.
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

// MCPSecretChangeType gives a secret update exact three-state semantics:
// omission preserves, set replaces, and clear removes.
type MCPSecretChangeType string

const (
	MCPSecretSet   MCPSecretChangeType = "set"
	MCPSecretClear MCPSecretChangeType = "clear"
)

// MCPAuthorizationChange is the write-only authorization change union.
type MCPAuthorizationChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value string              `json:"value,omitempty"`
}

// MCPHeadersChange is the write-only full replacement for HTTP headers. Header
// values may contain credentials, so reads expose masked values and updates use
// the same exact omission/set/clear semantics as Authorization. The map cannot
// contain Authorization itself; callers use [MCPAuthorizationChange] so that
// credential has one owner and one replacement path.
type MCPHeadersChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

// MCPEnvironmentChange is the write-only full replacement for a stdio
// process's environment. Environment values may contain credentials, so reads
// expose masked values and updates use exact omission/set/clear semantics.
type MCPEnvironmentChange struct {
	Type  MCPSecretChangeType `json:"type"`
	Value map[string]string   `json:"value,omitempty"`
}

// MCPConnectionInput is the write union for a complete connection descriptor.
// A connection replacement is atomic: fields from the other transport cannot
// survive a transport switch.
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

// MCPServerCandidate is a complete, unpersisted MCP server descriptor. Create
// persists it; test probes it without changing durable or live state.
type MCPServerCandidate struct {
	Name             string              `json:"name"`
	Enabled          bool                `json:"enabled"`
	Description      string              `json:"description,omitempty"`
	Connection       MCPConnectionInput  `json:"connection"`
	HandshakeTimeout MCPHandshakeTimeout `json:"handshakeTimeout"`
}

// UpdateMCPServerRequest saves configuration; enabled servers then connect in
// the background. Saving does not prove connection readiness. Omitted members preserve
// their current value; present empty strings, collections, and zeroes clear it.
// The identity is immutable and addressed by Server; installation servers
// refuse the update with an ownership error.
type UpdateMCPServerRequest struct {
	Server           MCPServerID          `json:"server"`
	Enabled          *bool                `json:"enabled,omitzero"`
	Description      *string              `json:"description,omitzero"`
	Connection       *MCPConnectionInput  `json:"connection,omitzero"`
	HandshakeTimeout *MCPHandshakeTimeout `json:"handshakeTimeout,omitzero"`
}

// MCPTool is one tool exposed by an MCP server. Tool list results are ordered by
// Server ascending and then Name ascending.
type MCPTool struct {
	ModelName string `json:"modelName"`
	// NameConflicts lists identities that exclude this tool from current model manifests.
	NameConflicts []ToolRef      `json:"nameConflicts"`
	Server        MCPServerID    `json:"server"`
	Name          string         `json:"name"`
	Description   string         `json:"description,omitempty"`
	InputSchema   map[string]any `json:"inputSchema,omitempty"`
}

// MCPTestOutcome is the closed verdict of mcp.servers.test. A probe carries no
// server-authored prose, so clients render each outcome locally and treat any
// other value as a contract violation.
type MCPTestOutcome string

const (
	MCPTestReachable             MCPTestOutcome = "reachable"
	MCPTestAuthorizationRequired MCPTestOutcome = "authorizationRequired"
	MCPTestTimedOut              MCPTestOutcome = "timedOut"
	MCPTestFailed                MCPTestOutcome = "failed"
)

// MCPTestResult is the semantic result of mcp.servers.test.
type MCPTestResult struct {
	Outcome MCPTestOutcome `json:"outcome"`
}

// MCPAuthorizationAttemptStatusType is the complete lifecycle of one
// interactive MCP OAuth flow.
type MCPAuthorizationAttemptStatusType string

const (
	MCPAuthorizationAttemptPending   MCPAuthorizationAttemptStatusType = "pending"
	MCPAuthorizationAttemptSucceeded MCPAuthorizationAttemptStatusType = "succeeded"
	MCPAuthorizationAttemptFailed    MCPAuthorizationAttemptStatusType = "failed"
	MCPAuthorizationAttemptCanceled  MCPAuthorizationAttemptStatusType = "canceled"
)

// MCPAuthorizationAttemptStatus is a closed union. Only failed carries an
// error, always mcp_authorization_failed; the full provider/OAuth error
// remains private telemetry.
type MCPAuthorizationAttemptStatus struct {
	Type  MCPAuthorizationAttemptStatusType `json:"type"`
	Error *MCPStatusProblem                 `json:"error,omitzero"`
}

// MCPAuthorizationAttempt is the observable asynchronous result of interactive
// authorization. Pending has no finishedAt; every terminal status has one.
type MCPAuthorizationAttempt struct {
	ID         string                        `json:"id"`
	Server     MCPServerID                   `json:"server"`
	Status     MCPAuthorizationAttemptStatus `json:"status"`
	CreatedAt  time.Time                     `json:"createdAt,omitzero"`
	FinishedAt *time.Time                    `json:"finishedAt,omitzero"`
}

// MCPToolExposure is the user-owned set hidden from model manifests.
type MCPToolExposure struct {
	Server        MCPServerID `json:"server"`
	DisabledTools []string    `json:"disabledTools"`
}
type SetMCPToolExposureRequest struct {
	Server   MCPServerID `json:"server"`
	Name     string      `json:"name"`
	Disabled bool        `json:"disabled"`
}
