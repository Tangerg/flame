package mcpserver

import (
	"errors"
	"fmt"
	"regexp"
)

const (
	// MaximumServerNameCharacters keeps the stable registry key compact enough
	// to leave meaningful space in the 64-byte model-facing "server_tool"
	// namespace. Server names are ASCII, so bytes and characters are identical.
	MaximumServerNameCharacters = 32
	serverNameAlphabet          = `[a-z0-9._-]`
)

var (
	// ServerNamePattern is the exact public and durable spelling of one MCP
	// server identity. Names are canonical lowercase slugs: no trimming,
	// normalization, case folding, or lossy sanitization is performed.
	ServerNamePattern = fmt.Sprintf(
		`^[a-z0-9]%s{0,%d}$`,
		serverNameAlphabet,
		MaximumServerNameCharacters-1,
	)
	serverNameExpression = regexp.MustCompile(ServerNamePattern)

	ErrInvalidServerName = errors.New("mcpserver: invalid server identity")
)

// ServerName is one exact user-chosen MCP registry identity. The same value
// owns persistence, live connection supersession, OAuth credentials, policy,
// and tool namespacing; it is not a display label.
type ServerName struct{ text string }

// ParseServerName admits only the canonical identity spelling.
func ParseServerName(raw string) (ServerName, error) {
	if !serverNameExpression.MatchString(raw) {
		return ServerName{}, fmt.Errorf(
			"%w: must match %s and contain at most %d characters",
			ErrInvalidServerName,
			ServerNamePattern,
			MaximumServerNameCharacters,
		)
	}
	return ServerName{text: raw}, nil
}

func (n ServerName) String() string { return n.text }

func (n ServerName) Validate() error {
	_, err := ParseServerName(n.text)
	return err
}
