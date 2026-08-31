package mcpserver

import (
	"errors"
	"fmt"
	"regexp"
)

// MaximumRemoteToolNameCharacters is the MCP protocol ceiling for one
// server-advertised tool identity. The alphabet is ASCII, so the character
// and encoded-byte bounds are identical.
const MaximumRemoteToolNameCharacters = 128

var (
	// RemoteToolNamePattern is the exact MCP tool-name grammar shared by live
	// discovery, durable policy configuration, and the public contract.
	RemoteToolNamePattern = fmt.Sprintf(
		`^[A-Za-z0-9_.-]{1,%d}$`,
		MaximumRemoteToolNameCharacters,
	)
	remoteToolNameExpression = regexp.MustCompile(RemoteToolNamePattern)

	// ErrInvalidRemoteToolName reports a remote identity that cannot be called
	// through the MCP protocol or used as a stable policy key.
	ErrInvalidRemoteToolName = errors.New("mcp: invalid remote tool name")
)

// RemoteToolName is the unchanged, case-sensitive identity advertised by one
// MCP server. It is deliberately distinct from the lossy, provider-facing
// function name produced by [ToolName].
type RemoteToolName struct {
	value string
}

// ParseRemoteToolName admits one exact MCP tool identity. It never trims,
// folds case, repairs punctuation, or truncates.
func ParseRemoteToolName(raw string) (RemoteToolName, error) {
	if !remoteToolNameExpression.MatchString(raw) {
		return RemoteToolName{}, fmt.Errorf(
			"%w: %q must match %s and contain at most %d characters",
			ErrInvalidRemoteToolName,
			raw,
			RemoteToolNamePattern,
			MaximumRemoteToolNameCharacters,
		)
	}
	return RemoteToolName{value: raw}, nil
}

// Validate reports whether name is an admitted remote identity.
func (n RemoteToolName) Validate() error {
	if !remoteToolNameExpression.MatchString(n.value) {
		return fmt.Errorf("%w: zero or malformed value", ErrInvalidRemoteToolName)
	}
	return nil
}

// String projects the exact MCP spelling at an SDK, wire, or storage boundary.
func (n RemoteToolName) String() string { return n.value }
