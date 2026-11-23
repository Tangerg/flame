package mcp

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

var errSessionEnded = errors.New("mcp: backend session ended")

func (c *connections) observeSessionEnd(session *sdk.ClientSession, owned *ownedSession) {
	defer c.observers.Done()
	err := session.Wait()
	c.mu.Lock()
	defer c.mu.Unlock()
	owned.end.err = err
	close(owned.end.done)
	c.retireSessionLocked(session)
	for _, configured := range c.servers {
		if configured.session != session {
			continue
		}
		configured.markFailed(mcpserver.FailureConnection, errors.Join(errSessionEnded, err))
		c.publishToolsLocked(configured.id)
	}
}
