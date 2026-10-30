package main

import (
	"errors"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
)

type agentSpec struct {
	Name              string   `json:"name"`
	CardURL           string   `json:"cardURL"`
	AllowedRPCOrigins []string `json:"allowedRPCOrigins,omitempty"`
	Description       string   `json:"description,omitempty"`
}

type agent struct {
	info       wire.Agent
	cardURL    string
	cardOrigin httporigin.Origin
	origins    []httporigin.Origin
	authority  identity.Digest
}

var agentName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func configureAgents(specs []agentSpec) (map[string]agent, error) {
	if len(specs) > 32 {
		return nil, errors.New("a2a: at most 32 agents may be configured")
	}
	agents := make(map[string]agent, len(specs))
	for _, spec := range specs {
		if !agentName.MatchString(spec.Name) || !utf8.ValidString(spec.Description) || len(spec.Description) > 4096 {
			return nil, errors.New("a2a: invalid agent name or description")
		}
		if _, duplicate := agents[spec.Name]; duplicate {
			return nil, errors.New("a2a: duplicate agent name")
		}
		u, origin, err := parseEndpoint(spec.CardURL)
		if err != nil {
			return nil, err
		}
		if len(spec.AllowedRPCOrigins) > 32 {
			return nil, errors.New("a2a: at most 32 RPC origins may be configured per agent")
		}
		a := agent{info: wire.Agent{Name: spec.Name, Description: spec.Description}, cardURL: u.String(), cardOrigin: origin, origins: []httporigin.Origin{origin}}
		for _, raw := range spec.AllowedRPCOrigins {
			u, origin, err := parseEndpoint(raw)
			if err != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery {
				return nil, errors.New("a2a: RPC origins must contain only scheme and authority")
			}
			a.origins = append(a.origins, origin)
		}
		slices.SortFunc(a.origins, func(a, b httporigin.Origin) int { return strings.Compare(a.String(), b.String()) })
		a.origins = slices.Compact(a.origins)
		identityParts := []string{a.cardURL}
		for _, origin := range a.origins {
			identityParts = append(identityParts, origin.String())
		}
		a.authority = identity.DigestStrings(identityParts...)
		agents[spec.Name] = a
	}
	return agents, nil
}

func parseEndpoint(raw string) (*url.URL, httporigin.Origin, error) {
	if len(raw) == 0 || len(raw) > 8192 || strings.TrimSpace(raw) != raw {
		return nil, httporigin.Origin{}, errors.New("a2a: invalid endpoint URL")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Fragment != "" {
		return nil, httporigin.Origin{}, errors.New("a2a: invalid endpoint URL")
	}
	origin, err := httporigin.FromURL(u)
	if err != nil {
		return nil, httporigin.Origin{}, errors.New("a2a: endpoint requires an HTTP(S) origin without URL credentials")
	}
	return u, origin, nil
}
