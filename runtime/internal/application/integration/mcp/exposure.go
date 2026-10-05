package mcp

import (
	"context"
	"fmt"
	"maps"
	"sync/atomic"

	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type exposureSnapshot struct {
	enabled  map[mcpserver.ID]bool
	disabled map[tool.Ref]bool
}

// ExposureState is a read-only projection of enabled sources and their separate
// exposure relation. Only MCP application mutations publish replacements.
type ExposureState struct {
	snapshot atomic.Pointer[exposureSnapshot]
}

func NewExposureState(servers []mcpserver.Server, disabled []tool.Ref) *ExposureState {
	state := &ExposureState{}
	snapshot := &exposureSnapshot{enabled: make(map[mcpserver.ID]bool), disabled: make(map[tool.Ref]bool)}
	for _, server := range servers {
		snapshot.enabled[server.ID()] = server.Enabled
	}
	for _, ref := range disabled {
		snapshot.disabled[ref] = true
	}
	state.snapshot.Store(snapshot)
	return state
}
func (s *ExposureState) ToolDisabled(ref tool.Ref) bool {
	snapshot := s.snapshot.Load()
	server, _, ok := ref.MCP()
	return snapshot == nil || !ok || !snapshot.enabled[server] || snapshot.disabled[ref]
}

// The coordinator serializes these projection updates with durable writes.
func (s *ExposureState) next() *exposureSnapshot {
	prior := s.snapshot.Load()
	return &exposureSnapshot{enabled: maps.Clone(prior.enabled), disabled: maps.Clone(prior.disabled)}
}

func (s *ExposureState) setServer(server mcpserver.Server) {
	next := s.next()
	next.enabled[server.ID()] = server.Enabled
	s.snapshot.Store(next)
}

func (s *ExposureState) removeServer(server mcpserver.ID) {
	next := s.next()
	delete(next.enabled, server)
	for ref := range next.disabled {
		if source, _, _ := ref.MCP(); source == server {
			delete(next.disabled, ref)
		}
	}
	s.snapshot.Store(next)
}

func (s *ExposureState) setToolDisabled(ref tool.Ref, disabled bool) {
	next := s.next()
	if disabled {
		next.disabled[ref] = true
	} else {
		delete(next.disabled, ref)
	}
	s.snapshot.Store(next)
}

func (c *Coordinator) ToolExposure(ctx context.Context, server mcpserver.ID) ([]tool.Ref, error) {
	if _, found, err := c.registry.Definition(ctx, server); err != nil {
		return nil, err
	} else if !found {
		return nil, ErrUnknownServer
	}
	refs, err := c.store.ListExposure(ctx)
	if err != nil {
		return nil, err
	}
	var selected []tool.Ref
	for _, ref := range refs {
		if source, _, _ := ref.MCP(); source == server {
			selected = append(selected, ref)
		}
	}
	return selected, nil
}

func (c *Coordinator) SetToolExposure(ctx context.Context, ref tool.Ref, disabled bool) error {
	server, _, ok := ref.MCP()
	if !ok {
		return fmt.Errorf("%w: exposure requires an MCP reference", ErrInvalidServerConfiguration)
	}
	write, err := c.beginMutation(ctx)
	if err != nil {
		return err
	}
	defer write.close()
	if _, found, err := c.registry.Definition(write.requestCtx, server); err != nil {
		return err
	} else if !found {
		return ErrUnknownServer
	}
	if err := c.store.SetToolExposure(write.requestCtx, ref, disabled); err != nil {
		return err
	}
	c.exposure.setToolDisabled(ref, disabled)
	write.unlock()
	c.invalidations.Notify(invalidation.Notice{Resource: invalidation.MCP})
	return nil
}
