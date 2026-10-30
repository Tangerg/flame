package mcpserver

import (
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu/integrity"
)

// AuthorityFingerprint excludes credentials and connection lifecycle settings:
// a token rotation does not grant authority to a different endpoint. Every slot
// is always present and the variable-length Args come last behind their count,
// so two different descriptors cannot frame to the same field sequence.
func (s Server) AuthorityFingerprint() integrity.Digest {
	var authority string
	if _, release, found := s.Source.Release(); found {
		authority = release.String()
	}
	fields := []string{string(s.Transport()), authority}
	return identity.DigestStrings(append(fields, s.Connection.authorityFields()...)...)
}

// SameConnection compares the descriptor realized by an executable. Enablement
// is checked separately at dispatch; notes and future handshake limits cannot
// change an existing connection.
func (s Server) SameConnection(other Server) bool {
	return s.Source == other.Source && s.Name == other.Name && s.Connection != nil && s.Connection.sameAs(other.Connection)
}
