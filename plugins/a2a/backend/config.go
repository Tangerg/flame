package main

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"

	framevalue "github.com/Tangerg/frame/value"
)

type agentSpec struct {
	Name              string   `json:"name"`
	CardURL           string   `json:"cardUrl"`
	AllowedRPCOrigins []string `json:"allowedRpcOrigins,omitempty"`
	Description       string   `json:"description,omitempty"`
}

type agent struct {
	info       wire.Agent
	cardURL    string
	cardOrigin httporigin.Origin
	origins    []httporigin.Origin
	authority  framevalue.Digest
}

var agentName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

const (
	maxAgents                = 32
	maxAgentRPCOrigins       = 32
	maxAgentDescriptionBytes = 4096
)

func configureAgents(specs []agentSpec) (map[string]agent, error) {
	if len(specs) > maxAgents {
		return nil, fmt.Errorf("a2a: at most %d agents may be configured", maxAgents)
	}
	agents := make(map[string]agent, len(specs))
	for _, spec := range specs {
		if _, duplicate := agents[spec.Name]; duplicate {
			return nil, errors.New("a2a: duplicate agent name")
		}
		configured, err := newAgent(spec)
		if err != nil {
			return nil, err
		}
		agents[spec.Name] = configured
	}
	return agents, nil
}

func newAgent(spec agentSpec) (agent, error) {
	if !agentName.MatchString(spec.Name) || !utf8.ValidString(spec.Description) || len(spec.Description) > maxAgentDescriptionBytes {
		return agent{}, errors.New("a2a: invalid agent name or description")
	}
	u, origin, err := parseEndpoint(spec.CardURL)
	if err != nil {
		return agent{}, err
	}
	if len(spec.AllowedRPCOrigins) > maxAgentRPCOrigins {
		return agent{}, fmt.Errorf("a2a: at most %d RPC origins may be configured per agent", maxAgentRPCOrigins)
	}
	a := agent{info: wire.Agent{Name: spec.Name, Description: spec.Description}, cardURL: u.String(), cardOrigin: origin, origins: []httporigin.Origin{origin}}
	for _, raw := range spec.AllowedRPCOrigins {
		u, origin, err := parseEndpoint(raw)
		if err != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery {
			return agent{}, errors.New("a2a: RPC origins must contain only scheme and authority")
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
	return a, nil
}

const maxEndpointBytes = 8192

func parseEndpoint(raw string) (*url.URL, httporigin.Origin, error) {
	if len(raw) == 0 || len(raw) > maxEndpointBytes || strings.TrimSpace(raw) != raw {
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
