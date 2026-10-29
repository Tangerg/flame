package mcp

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func (c *connections) toolListChanged(_ context.Context, request *sdk.ToolListChangedRequest) {
	session := request.Session
	c.mu.Lock()
	defer c.mu.Unlock()
	owned := c.sessions[session]
	if c.closed || owned == nil || owned.close != nil || owned.toolChanges == nil {
		return
	}
	owned.toolVersion++
	select {
	case owned.toolChanges <- struct{}{}:
	default:
	}
}

// Notification versions invalidate captured catalogs before any refresh I/O.
// Each session has one worker; a later notification discards an older result.
func (c *connections) watchToolChangesLocked(session *sdk.ClientSession, owned *ownedSession, config ServerConfig, publishedVersion uint64) {
	ctx, cancel := context.WithCancel(c.lifetime)
	owned.cancelTools = cancel
	c.observers.Add(1)
	go func() {
		defer c.observers.Done()
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-owned.toolChanges:
			}
			c.mu.Lock()
			target := c.find(config.ID())
			if c.closed || target == nil || target.session != session || c.sessions[session] != owned || owned.close != nil {
				c.mu.Unlock()
				return
			}
			version := owned.toolVersion
			c.mu.Unlock()
			if version == publishedVersion {
				continue
			}
			tools, err := sourceTools(ctx, c, config, session)
			c.mu.Lock()
			target = c.find(config.ID())
			if c.closed || ctx.Err() != nil || target == nil || target.session != session || c.sessions[session] != owned || owned.close != nil {
				c.mu.Unlock()
				return
			}
			if owned.toolVersion != version {
				c.mu.Unlock()
				continue
			}
			if err != nil {
				target.settle(nil, nil, mcpserver.ConnectionFailed, mcpserver.FailureToolDiscovery, err)
				c.retireSessionLocked(session)
			} else {
				for index := range tools {
					tools[index].catalogVersion = version
				}
				target.settle(session, tools, mcpserver.ConnectionConnected, "", nil)
				publishedVersion = version
			}
			c.publishToolsLocked(config.ID())
			c.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
}
