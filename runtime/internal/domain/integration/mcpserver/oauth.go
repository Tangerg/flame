package mcpserver

import (
	"errors"
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	framevalue "github.com/Tangerg/frame/value"
)

var ErrOAuthSessionSuperseded = errors.New("mcpserver: oauth session superseded")

type OAuthTarget struct {
	Source  Source
	Name    ServerName
	URL     string
	Headers map[string]string
}

func (o OAuthTarget) ID() ID { return o.Source.ID(o.Name) }

func (o OAuthTarget) Fingerprint() framevalue.Digest {
	installation, installed := o.Source.Origin().Installation()
	fields := []string{string(o.Source.Origin().Kind()), installation.String(), o.Name.String()}
	if installed {
		return identity.DigestStrings(append(fields, o.Source.recipient.String())...)
	}
	fields = append(fields, o.Source.connector.String(), o.URL)
	for _, name := range slices.Sorted(maps.Keys(o.Headers)) {
		fields = append(fields, name, o.Headers[name])
	}
	return identity.DigestStrings(fields...)
}

func (s Server) OAuthTarget() (OAuthTarget, bool) {
	connection, ok := s.Connection.(HTTPConnection)
	if !ok || connection.Authorization != "" {
		return OAuthTarget{}, false
	}
	return OAuthTarget{Source: s.Source, Name: s.Name, URL: connection.URL, Headers: maps.Clone(connection.Headers)}, true
}
