package delivery

import (
	"context"
	"encoding/base64"
	"mime"
	"path"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

func (s *Handler) ReadPluginResource(ctx context.Context, in protocol.ReadPluginResourceRequest) (*protocol.PluginResource, error) {
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
