package mcpserver

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

const (
	// MaximumServerNameCharacters keeps the local name compact enough to leave
	// meaningful space in the 64-byte model-facing "server_tool" namespace.
	// Server names are ASCII, so bytes and characters are identical.
	MaximumServerNameCharacters = 32
	serverNameAlphabet          = `[a-z0-9._-]`
)

var (
	serverNameExpression = regexp.MustCompile(fmt.Sprintf(
		`^[a-z0-9]%s{0,%d}$`,
		serverNameAlphabet,
		MaximumServerNameCharacters-1,
	))

	ErrInvalidServerName = errors.New("mcpserver: invalid server name")
	ErrInvalidOrigin     = errors.New("mcpserver: invalid server origin")
)

// ServerNamePattern projects the canonical public and durable spelling without
// exposing a separate mutable grammar to consumers.
func ServerNamePattern() string { return serverNameExpression.String() }

// ServerName is the name a record's origin owner chose for it. It is unique
// only within that origin; [ID] is the registry identity.
type ServerName struct {
	text string
}

// ParseServerName admits only the canonical name spelling.
func ParseServerName(raw string) (ServerName, error) {
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

func (n ServerName) String() string { return n.text }

// MarshalText and UnmarshalText let a stored declaration carry the canonical
// spelling; decoding parses it again, so no unparsed name is ever admitted.
func (n ServerName) MarshalText() ([]byte, error) { return []byte(n.text), nil }

func (n *ServerName) UnmarshalText(text []byte) error {
	parsed, err := ParseServerName(string(text))
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}

// Validate reports whether the name was parsed. Its exact spelling is
// established there, so an unconstructed name is all this can reject.
func (n ServerName) Validate() error {
	if n.text == "" {
		return ErrInvalidServerName
	}
	return nil
}

// OriginKind is the closed set of owners that may advance a registry record.
type OriginKind string

const (
	OriginUser         OriginKind = "user"
	OriginInstallation OriginKind = "installation"
)

// Origin names the only owner allowed to advance a record's descriptor and
// enablement: the user's MCP registry, or one installation. The zero value is
// invalid, so an origin is always an explicit choice.
type Origin struct {
	kind         OriginKind
	installation resourceid.InstallationID
}

func UserOrigin() Origin { return Origin{kind: OriginUser} }

func InstallationOrigin(installation resourceid.InstallationID) (Origin, error) {
	if err := installation.Validate(); err != nil {
		return Origin{}, fmt.Errorf("%w: %w", ErrInvalidOrigin, err)
	}
	return Origin{kind: OriginInstallation, installation: installation}, nil
}

func (o Origin) Kind() OriginKind { return o.kind }

func (o Origin) Installation() (resourceid.InstallationID, bool) {
	return o.installation, o.kind == OriginInstallation
}

func (o Origin) rank() int {
	if o.kind == OriginUser {
		return 0
	}
	return 1
}

func (o Origin) Validate() error {
	if o.kind == "" {
		return fmt.Errorf("%w: origin is unset", ErrInvalidOrigin)
	}
	return nil
}

// ID is the exact registry identity of one MCP server. The same value owns
// persistence, live connection supersession, OAuth credentials, tool policy
// and the source half of every MCP tool reference; it is not a display label.
type ID struct {
	origin Origin
	name   ServerName
}

func NewID(origin Origin, name ServerName) (ID, error) {
	if err := origin.Validate(); err != nil {
		return ID{}, fmt.Errorf("mcpserver: server identity: %w", err)
	}
	if err := name.Validate(); err != nil {
		return ID{}, fmt.Errorf("mcpserver: server identity: %w", err)
	}
	return ID{origin: origin, name: name}, nil
}

func (i ID) Origin() Origin   { return i.origin }
func (i ID) Name() ServerName { return i.name }

func (i ID) Validate() error {
	if err := i.origin.Validate(); err != nil {
		return fmt.Errorf("mcpserver: server identity: %w", err)
	}
	if err := i.name.Validate(); err != nil {
		return fmt.Errorf("mcpserver: server identity: %w", err)
	}
	return nil
}

// Compare orders user servers before installation servers, then by
// installation and name, so every listing has one deterministic order.
func (i ID) Compare(other ID) int {
	return cmp.Or(
		cmp.Compare(i.origin.rank(), other.origin.rank()),
		cmp.Compare(i.origin.installation.String(), other.origin.installation.String()),
		cmp.Compare(i.name.text, other.name.text),
	)
}

// String renders the identity for diagnostics only. Every boundary carries the
// structured value; no reader parses this text.
func (i ID) String() string {
	if installation, ok := i.origin.Installation(); ok {
		return "installation " + installation.String() + " server " + i.name.text
	}
	return i.name.text
}

// Source is the owner of one registry record. An installation source also
// carries the admitted release the record realizes, the tool authority of its
// code and the recipient its credentials follow, so none of them can be
// claimed by a user record or omitted from an installation record.
type Source struct {
	origin    Origin
	release   fingerprint.Digest
	authority fingerprint.Digest
	recipient fingerprint.Digest
}

func UserSource() Source { return Source{origin: UserOrigin()} }

func InstallationSource(installation resourceid.InstallationID, release, authority, recipient fingerprint.Digest) (Source, error) {
	origin, err := InstallationOrigin(installation)
	if err != nil {
		return Source{}, fmt.Errorf("mcpserver: installation source: %w", err)
	}
	if err := release.Validate(); err != nil {
		return Source{}, fmt.Errorf("%w: release: %w", ErrInvalidOrigin, err)
	}
	if err := authority.Validate(); err != nil {
		return Source{}, fmt.Errorf("%w: authority: %w", ErrInvalidOrigin, err)
	}
	if err := recipient.Validate(); err != nil {
		return Source{}, fmt.Errorf("%w: credential recipient: %w", ErrInvalidOrigin, err)
	}
	return Source{origin: origin, release: release, authority: authority, recipient: recipient}, nil
}

func (s Source) Origin() Origin { return s.origin }

// ID names the record this source owns under name. It is valid exactly when
// the source and name are.
func (s Source) ID(name ServerName) ID { return ID{origin: s.origin, name: name} }

// Release reports the admitted release and tool authority of an installation
// source.
func (s Source) Release() (release, authority fingerprint.Digest, found bool) {
	return s.release, s.authority, s.origin.kind == OriginInstallation
}

func (s Source) Validate() error { return s.origin.Validate() }
