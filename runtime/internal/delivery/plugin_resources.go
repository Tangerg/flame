package delivery

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"iter"
	"mime"
	"path"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

type resourceLease struct {
	owner   context.Context
	request protocol.OpenPluginResourcesRequest
}

// Resource authority exists only for the live subscription. Its ephemeral
// capability cannot become installation state or a retained execution dependency.
type resourceLeases struct {
	mu     sync.Mutex
	values map[string]resourceLease
}

func newResourceLeases() *resourceLeases { return &resourceLeases{values: map[string]resourceLease{}} }

func (r *resourceLeases) open(ctx context.Context, request protocol.OpenPluginResourcesRequest) (string, func()) {
	token := rand.Text()
	r.mu.Lock()
	r.values[token] = resourceLease{owner: ctx, request: request}
	r.mu.Unlock()
	close := func() { r.mu.Lock(); delete(r.values, token); r.mu.Unlock() }
	stop := context.AfterFunc(ctx, close)
	return token, func() { stop(); close() }
}

func (s *Handler) resourceLease(token string) (resourceLease, bool) {
	if s.resources == nil {
		return resourceLease{}, false
	}
	s.resources.mu.Lock()
	defer s.resources.mu.Unlock()
	value, found := s.resources.values[token]
	return value, found && value.owner.Err() == nil
}

func (s *Handler) OpenPluginResources(ctx context.Context, in protocol.OpenPluginResourcesRequest) (*protocol.PluginResourceLease, iter.Seq2[protocol.PluginResourceEvent, error], error) {
	if s.plugins == nil || s.resources == nil {
		return nil, nil, protocol.ErrPluginUnavailable
	}
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, nil, err
	}
	component, err := plugin.ParseContributionID(in.ComponentID)
	if err != nil {
		return nil, nil, protocol.InvalidParameters(err)
	}
	value, err := s.plugins.Component(ctx, id, digest, component)
	if err != nil {
		return nil, nil, wirePluginError(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	token, close := s.resources.open(ctx, in)
	events := func(func(protocol.PluginResourceEvent, error) bool) { defer close(); <-ctx.Done() }
	return &protocol.PluginResourceLease{Token: token, Entry: value.Entry}, events, nil
}

func (s *Handler) ReadPluginResource(ctx context.Context, in protocol.ReadPluginResourceRequest) (*protocol.PluginResource, error) {
	if s.plugins == nil {
		return nil, protocol.ErrPluginUnavailable
	}
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	component, err := plugin.ParseContributionID(in.ComponentID)
	if err != nil {
		return nil, protocol.InvalidParameters(err)
	}
	body, err := s.plugins.Resource(ctx, id, digest, component, in.Path)
	if err != nil {
		return nil, wirePluginError(err)
	}
	mediaType := mime.TypeByExtension(path.Ext(in.Path))
	if mediaType == "" {
		mediaType = "application/octet-stream"
	}
	return &protocol.PluginResource{Body: base64.StdEncoding.EncodeToString(body), MediaType: mediaType}, nil
}

func (s *Handler) ReadPluginLocale(ctx context.Context, in protocol.ReadPluginLocaleRequest) (*protocol.PluginLocaleCatalog, error) {
	if s.plugins == nil {
		return nil, protocol.ErrPluginUnavailable
	}
	id, digest, err := parseInstallationRelease(in.InstallationID, in.Digest)
	if err != nil {
		return nil, err
	}
	messages, err := s.plugins.Locale(ctx, id, digest, in.Tag)
	if err != nil {
		return nil, wirePluginError(err)
	}
	return &protocol.PluginLocaleCatalog{Messages: messages}, nil
}

// LeasedPluginResource translates an asset request into the same public Core
// query with its captured caller and subscription lifetime.
func (e *Endpoint) LeasedPluginResource(ctx context.Context, token, resource string) (*protocol.PluginResource, error) {
	owner, ok := e.target.(interface {
		resourceLease(string) (resourceLease, bool)
	})
	if !ok {
		return nil, protocol.ErrPluginUnavailable
	}
	lease, found := owner.resourceLease(token)
	if !found {
		return nil, protocol.ErrPluginUnavailable
	}
	owned, cancel := context.WithCancel(lease.owner)
	defer cancel()
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	meta, _ := RequestMetaFrom(lease.owner)
	return e.Call[protocol.ReadPluginResourceRequest, *protocol.PluginResource](owned, PluginsReadResource, protocol.ReadPluginResourceRequest{OpenPluginResourcesRequest: lease.request, Path: resource}, Options{RequestMeta: meta})
}
