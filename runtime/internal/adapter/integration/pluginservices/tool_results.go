package pluginservices

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/runtime/internal/adapter/integration/mcp"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/scope/core/chat"
	"github.com/samber/lo"
)

type ToolResults struct {
	installations plugins.Store
	pool          *mcp.Pool
}

func (s *ToolResults) OwnsToolSource(ctx context.Context, ref tool.Ref, release identity.Digest, activation plugin.Activation) (bool, error) {
	_, found, err := s.toolSource(ctx, ref, release, activation)
	return found, err
}

func (s *ToolResults) toolSource(ctx context.Context, ref tool.Ref, release identity.Digest, activation plugin.Activation) (mcpserver.ID, bool, error) {
	var zero mcpserver.ID
	server, _, found := ref.MCP()
	if !found {
		return zero, false, nil
	}
	installation, managed := server.Origin().Installation()
	if !managed {
		return zero, false, nil
	}
	if _, err := plugins.OriginalActivation([]plugin.Activation{activation}, installation, release, server.Name()); err != nil {
		return zero, false, err
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

func (s *ToolResults) OriginalToolResult(ctx context.Context, ref tool.Ref, release, authority identity.Digest, activation plugin.Activation, callID, arguments string) (_ *chat.ToolOutput, err error) {
	borrowed, err := s.originalToolSession(ctx, ref, release, authority, activation)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, borrowed.Close()) }()
	ctx = borrowed.Context()
	session := borrowed.Session()
	admitted, err := jsonvalue.Parse([]byte(arguments))
	if err != nil {
		return nil, err
	}
	_, remote, _ := ref.MCP()
	return pluginsdk.ReadToolReceipt(ctx, session, callID, remote.String(), admitted)
}

func (s *ToolResults) AwaitOriginalToolSource(ctx context.Context, ref tool.Ref, release, authority identity.Digest, activation plugin.Activation) (bool, error) {
	if err := cancellation.Err(ctx); err != nil {
		return false, err
	}
	borrowed, err := s.originalToolSession(ctx, ref, release, authority, activation)
	if borrowed != nil {
		err = errors.Join(err, borrowed.Close())
	}
	if ctx.Err() == nil && (errors.Is(err, plugin.ErrUnavailable) || errors.Is(err, mcp.ErrUnavailable)) {
		return false, nil
	}
	return err == nil, err
}

func (s *ToolResults) originalToolSession(ctx context.Context, ref tool.Ref, release, authority identity.Digest, activation plugin.Activation) (*mcp.BorrowedSession, error) {
	source, found, err := s.toolSource(ctx, ref, release, activation)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, plugin.ErrUnavailable
	}
	borrowed, err := s.pool.BorrowActivation(ctx, source, activation)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.SessionActor(borrowed.Context(), source, borrowed.Session())
	if err != nil {
		return nil, errors.Join(err, borrowed.Close())
	}
	fingerprint, err := borrowed.SourceFingerprint()
	if err == nil && fingerprint != authority {
		err = plugin.ErrStale
	}
	if err != nil {
		return nil, errors.Join(err, borrowed.Close())
	}
	return borrowed, nil
}

func NewToolResults(store plugins.Store, pool *mcp.Pool) (*ToolResults, error) {
	if lo.IsNil(store) || pool == nil {
		return nil, errors.New("tool results: installation store and connections are required")
	}
	return &ToolResults{installations: store, pool: pool}, nil
}
