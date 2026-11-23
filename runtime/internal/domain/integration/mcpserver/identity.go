package mcpserver

import (
	"cmp"
	"errors"
	"fmt"
	"regexp"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	framevalue "github.com/Tangerg/frame/value"
)

const (
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

func ServerNamePattern() string { return serverNameExpression.String() }

type ServerName struct {
	text string
}

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

func (ServerName) JSONSchemaModel() any { return "" }

func (n ServerName) MarshalText() ([]byte, error) { return []byte(n.text), nil }

func (n *ServerName) UnmarshalText(text []byte) error {
	parsed, err := ParseServerName(string(text))
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}

func (n ServerName) Validate() error {
	if n.text == "" {
		return ErrInvalidServerName
	}
	return nil
}

type OriginKind string

const (
	OriginUser         OriginKind = "user"
	OriginInstallation OriginKind = "installation"
)

type Origin struct {
	kind         OriginKind
	installation identity.InstallationID
}

func UserOrigin() Origin { return Origin{kind: OriginUser} }

func InstallationOrigin(installation identity.InstallationID) (Origin, error) {
	if err := installation.Validate(); err != nil {
		return Origin{}, fmt.Errorf("%w: %w", ErrInvalidOrigin, err)
	}
	return Origin{kind: OriginInstallation, installation: installation}, nil
}

func (o Origin) Kind() OriginKind { return o.kind }

func (o Origin) Installation() (identity.InstallationID, bool) {
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

func (i ID) Compare(other ID) int {
	return cmp.Or(
		cmp.Compare(i.origin.rank(), other.origin.rank()),
		cmp.Compare(i.origin.installation.String(), other.origin.installation.String()),
		cmp.Compare(i.name.text, other.name.text),
	)
}

func (i ID) String() string {
	if installation, ok := i.origin.Installation(); ok {
		return "installation " + installation.String() + " server " + i.name.text
	}
	return i.name.text
}

type Source struct {
	connector identity.InstallationID
	origin    Origin
	release   framevalue.Digest
	authority framevalue.Digest
	recipient framevalue.Digest
}

func UserSource() Source { return Source{origin: UserOrigin()} }

func ConnectorSource(id identity.InstallationID) (Source, error) {
	if err := id.Validate(); err != nil {
		return Source{}, err
	}
	return Source{origin: UserOrigin(), connector: id}, nil
}

func InstallationSource(installation identity.InstallationID, release, authority, recipient framevalue.Digest) (Source, error) {
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

func (s Source) ID(name ServerName) ID { return ID{origin: s.origin, name: name} }

func (s Source) Release() (release, authority framevalue.Digest, found bool) {
	return s.release, s.authority, s.origin.kind == OriginInstallation
}

func (s Source) Validate() error { return s.origin.Validate() }
