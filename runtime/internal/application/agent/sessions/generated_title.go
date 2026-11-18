package sessions

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/keylock"
)

type titleClaims struct {
	writes  *keylock.Set
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

// Title publication shares the commit boundary because an archive can reuse
// a Session and Run identity after replacing the history that issued the claim.
func (c *titleClaims) commitHistoryChange(ctx context.Context, sessionID string, removes func(runID string) bool, commit func() error) error {
	release, err := c.writes.Acquire(ctx, sessionID)
	if err != nil {
		return err
	}
	defer release()
	if err := commit(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if claim := c.pending[sessionID]; claim != nil && removes(claim.runID) {
		delete(c.pending, sessionID)
	}
	return nil
}

func (g *GeneratedTitle) persist(ctx context.Context, title string) (bool, error) {
	c := g.owner
	release, err := c.titles.writes.Acquire(ctx, g.sessionID)
	if err != nil {
		return false, err
	}
	defer release()
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
