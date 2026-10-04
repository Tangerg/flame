package mcpserver

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
)

const (
	// MaximumServerNameCharacters keeps the stable registry key compact enough
	// to leave meaningful space in the 64-byte model-facing "server_tool"
	// namespace. Server names are ASCII, so bytes and characters are identical.
	MaximumServerNameCharacters = 32
	serverNameAlphabet          = `[a-z0-9._-]`
	installationServerPrefix    = "installation/"
)

var (
	serverNameExpression = regexp.MustCompile(fmt.Sprintf(
		`^[a-z0-9]%s{0,%d}$`,
		serverNameAlphabet,
		MaximumServerNameCharacters-1,
	))

	ErrInvalidServerName = errors.New("mcpserver: invalid server identity")
)

// ServerNamePattern projects the canonical public and durable spelling without
// exposing a separate mutable grammar to consumers.
func ServerNamePattern() string { return serverNameExpression.String() }

func ServerIdentityPattern() string {
	return fmt.Sprintf(
		`^(?:%s%s/)?%s$`,
		regexp.QuoteMeta(installationServerPrefix),
		strings.Trim(resourceid.InstallationIDPattern(), "^$"),
		strings.Trim(ServerNamePattern(), "^$"),
	)
}

// ServerName is one exact user-chosen MCP registry identity. The same value
// owns persistence, live connection supersession, OAuth credentials, policy,
// and tool namespacing; it is not a display label.
type ServerName struct {
	text         string
	installation string
}

// ParseServerName admits only the canonical identity spelling.
func ParseServerName(raw string) (ServerName, error) {
	if strings.HasPrefix(raw, installationServerPrefix) {
		parts := strings.Split(raw, "/")
		if len(parts) != 3 {
			return ServerName{}, ErrInvalidServerName
		}
		return InstallationServer(parts[1], parts[2])
	}
	if !serverNameExpression.MatchString(raw) {
		return ServerName{}, fmt.Errorf(
			"%w: must match %s and contain at most %d characters",
			ErrInvalidServerName,
			ServerNamePattern(),
			MaximumServerNameCharacters,
		)
	}
	return ServerName{text: raw}, nil
}

func InstallationServer(installation, local string) (ServerName, error) {
	if _, err := resourceid.ParseInstallation(installation); err != nil {
		return ServerName{}, fmt.Errorf("%w: installation: %w", ErrInvalidServerName, err)
	}
	if !serverNameExpression.MatchString(local) {
		return ServerName{}, fmt.Errorf("%w: local name must match %s", ErrInvalidServerName, ServerNamePattern())
	}
	return ServerName{text: local, installation: installation}, nil
}

func (n ServerName) String() string {
	if n.installation != "" {
		return installationServerPrefix + n.installation + "/" + n.text
	}
	return n.text
}
func (n ServerName) Local() string        { return n.text }
func (n ServerName) Installation() string { return n.installation }

// Validate reports whether the name was parsed. Its exact spelling is
// established there, so an unconstructed name is all this can reject.
func (n ServerName) Validate() error {
	if n.text == "" {
		return ErrInvalidServerName
	}
	return nil
}
