package sessions

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type titleClaims struct {
	mu      sync.Mutex
	pending map[string]*GeneratedTitle
}

// GeneratedTitle can name the original untitled Session while its source
// history survives. Session imports can reuse the ID, so the ID alone cannot
// authorize a detached model result.
type GeneratedTitle struct {
	owner     *Coordinator
	sessionID string
	runID     string
}

// PrepareGeneratedTitle is called before the originating Run releases Session
// admission. Only one detached title request can name an untitled Session.
func (c *Coordinator) PrepareGeneratedTitle(ctx context.Context, sessionID, runID string) (*GeneratedTitle, error) {
	c.titles.mu.Lock()
	defer c.titles.mu.Unlock()
	if c.titles.pending[sessionID] != nil {
		return nil, nil
	}
	value, err := c.Get(ctx, sessionID)
	if err != nil || value.Title() != "" {
		return nil, err
	}
	claim := &GeneratedTitle{owner: c, sessionID: sessionID, runID: runID}
	if c.titles.pending == nil {
		c.titles.pending = make(map[string]*GeneratedTitle)
	}
	c.titles.pending[sessionID] = claim
	return claim, nil
}

func (g *GeneratedTitle) Apply(ctx context.Context, title string) error {
	changed, err := g.persist(ctx, title)
	if changed {
		g.owner.publishSessionMoved(g.sessionID)
	}
	return err
}

func (g *GeneratedTitle) Release() {
	c := g.owner
	c.titles.mu.Lock()
	defer c.titles.mu.Unlock()
	if c.titles.pending[g.sessionID] == g {
		delete(c.titles.pending, g.sessionID)
	}
}

func (c *Coordinator) retireGeneratedTitle(sessionID string) {
	c.titles.mu.Lock()
	defer c.titles.mu.Unlock()
	delete(c.titles.pending, sessionID)
}

func (c *Coordinator) retireRolledBackTitle(sessionID string, droppedRunIDs []string) {
	c.titles.mu.Lock()
	defer c.titles.mu.Unlock()
	if claim := c.titles.pending[sessionID]; claim != nil && slices.Contains(droppedRunIDs, claim.runID) {
		delete(c.titles.pending, sessionID)
	}
}

func (g *GeneratedTitle) persist(ctx context.Context, title string) (bool, error) {
	c := g.owner
	c.titles.mu.Lock()
	defer c.titles.mu.Unlock()
	if c.titles.pending[g.sessionID] != g {
		return false, nil
	}
	defer delete(c.titles.pending, g.sessionID)
	for {
		if err := ctx.Err(); err != nil {
			return false, err
		}
		current, err := c.Get(ctx, g.sessionID)
		if err != nil {
			return false, err
		}
		updated, changed, err := current.NameIfUntitled(title, c.now())
		if err != nil || !changed {
			return false, err
		}
		replacement, err := session.NextReplacement(current, updated)
		if err != nil {
			return false, fmt.Errorf("sessions: prepare generated title replacement: %w", err)
		}
		if err := c.sessions.Save(ctx, replacement); err == nil {
			return true, nil
		} else if !errors.Is(err, session.ErrRevisionConflict) {
			return false, fmt.Errorf("sessions: save generated title: %w", err)
		}
	}
}
