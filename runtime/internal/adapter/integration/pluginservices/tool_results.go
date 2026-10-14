package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcpconnection"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/cancellation"
	"github.com/Tangerg/flame/runtime/internal/dependency"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/mcp"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

type ToolResults struct {
	installations plugins.Store
	pool          *mcpconnection.Pool
}

func (s *ToolResults) OwnsToolSource(ctx context.Context, ref tool.Ref, release fingerprint.Digest) (bool, error) {
	_, found, err := s.toolSource(ctx, ref, release)
	return found, err
}

func (s *ToolResults) toolSource(ctx context.Context, ref tool.Ref, release fingerprint.Digest) (mcpserver.ID, bool, error) {
	var zero mcpserver.ID
	server, _, found := ref.MCP()
	if !found {
		return zero, false, nil
	}
	installation, managed := server.Origin().Installation()
	if !managed {
		return zero, false, nil
	}
	snapshot, found, err := s.installations.Get(ctx, installation)
	if err != nil || !found {
		return zero, false, err
	}
	if snapshot.Selected.Digest() != release {
		return zero, false, nil
	}
	for _, declared := range snapshot.Selected.Declaration().Servers {
		if !snapshot.Installation.ServerEnabled(declared.Name) {
			continue
		}
		id, err := snapshot.Installation.ServerID(declared.Name)
		if err != nil {
			return zero, false, err
		}
		if id == server {
			return server, true, nil
		}
	}
	return zero, false, nil
}

func (s *ToolResults) OriginalToolResult(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest, callID, arguments string) (*chat.ToolOutput, error) {
	session, err := s.originalToolSession(ctx, ref, release, authority)
	if err != nil {
		return nil, err
	}
	admitted, err := jsoninput.Parse([]byte(arguments))
	if err != nil {
		return nil, err
	}
	_, remote, _ := ref.MCP()
	return pluginsdk.ReadToolReceipt(ctx, session, callID, remote.String(), admitted)
}

func (s *ToolResults) AwaitOriginalToolSource(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest) (bool, error) {
	if err := cancellation.Err(ctx); err != nil {
		return false, err
	}
	_, err := s.originalToolSession(ctx, ref, release, authority)
	if ctx.Err() == nil && (errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, mcp.ErrUnavailable)) {
		return false, nil
	}
	return err == nil, err
}

func (s *ToolResults) originalToolSession(ctx context.Context, ref tool.Ref, release, authority fingerprint.Digest) (*sdk.ClientSession, error) {
	source, found, err := s.toolSource(ctx, ref, release)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, plugin.ErrUnavailable
	}
	return s.pool.BoundServiceSession(ctx, source, authority)
}

func NewToolResults(store plugins.Store, pool *mcpconnection.Pool) (*ToolResults, error) {
	if dependency.Missing(store) || pool == nil {
		return nil, errors.New("Tool results: installation store and connections are required")
	}
	return &ToolResults{installations: store, pool: pool}, nil
}
