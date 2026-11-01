package model

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/httporigin"
)

// EndpointPolicy describes the version boundary at a provider's gateway mount.
// A version-appending SDK may deliberately sit behind a versioned proxy path.
type EndpointPolicy struct {
	Example        string `json:"example"`
	AppendsPath    string `json:"appendsPath,omitzero"`
	RequiredSuffix string `json:"requiredSuffix,omitzero"`
}

var ErrEndpointOverride = errors.New("model provider: invalid endpoint override")

func (p EndpointPolicy) Validate() error {
	if p == (EndpointPolicy{}) {
		return nil
	}
	if p.Example == "" || p.AppendsPath != "" && p.RequiredSuffix != "" {
		return errors.New("model provider: endpoint policy requires one example and exclusive path rules")
	}
	for _, fragment := range []string{p.AppendsPath, p.RequiredSuffix} {
		if fragment != "" && (!strings.HasPrefix(fragment, "/") || fragment == "/" || path.Clean(fragment) != fragment || (&url.URL{Path: fragment}).EscapedPath() != fragment) {
			return errors.New("model provider: endpoint rule must name an absolute unescaped path")
		}
	}
	return p.ValidateOverride(p.Example)
}

func (p EndpointPolicy) ValidateOverride(raw string) error {
	if raw == "" {
		return nil
	}
	endpoint, err := httporigin.ParseEndpoint(raw)
	if err != nil || endpoint.HasFragment() || endpoint.HasQuery() {
		return ErrEndpointOverride
	}
	if p.RequiredSuffix != "" && !strings.HasSuffix(strings.TrimSuffix(endpoint.URL().Path, "/"), p.RequiredSuffix) {
		return fmt.Errorf("%w: URL path must end with %s", ErrEndpointOverride, p.RequiredSuffix)
	}
	return nil
}

// EffectivePath exposes only the route, so presenting a draft cannot echo URL credentials.
func (p EndpointPolicy) EffectivePath(raw string) string {
	if raw == "" {
		return ""
	}
	endpoint, err := httporigin.ParseEndpoint(raw)
	if err != nil || endpoint.HasQuery() || endpoint.HasFragment() {
		return ""
	}
	result := strings.TrimRight(endpoint.URL().EscapedPath(), "/") + p.AppendsPath
	if result == "" {
		return "/"
	}
	return result
}
