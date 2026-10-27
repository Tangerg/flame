package mcp

import (
	"context"
	"errors"
	"sync"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// BorrowedSession holds one call in the original Use. Context propagates
// withdrawal to the call; Close joins its work without closing the process.
type BorrowedSession struct {
	ctx     context.Context
	session *sdk.ClientSession
	work    *qimu.Work
	once    sync.Once
	managed *installedSession
	release func() error
	err     error
}

func (b *BorrowedSession) Context() context.Context    { return b.ctx }
func (b *BorrowedSession) Session() *sdk.ClientSession { return b.session }
func (b *BorrowedSession) Close() error {
	b.once.Do(func() {
		if b.work != nil {
			b.work.Finish()
		}
		if b.release != nil {
			b.err = b.release()
		}
	})
	return b.err
}

func (c *connections) borrowSession(ctx context.Context, session *sdk.ClientSession) (*BorrowedSession, error) {
	c.mu.Lock()
	owned := c.sessions[session]
	if c.closed || owned == nil || owned.close != nil {
		c.mu.Unlock()
		return nil, ErrUnavailable
	}
	observation := owned.managed
	c.mu.Unlock()
	if observation == nil {
		borrowed := &BorrowedSession{ctx: ctx, session: session}
		borrowed.ctx = context.WithValue(ctx, borrowContextKey{}, borrowed)
		return borrowed, nil
	}
	managed, err := observation.owner.acquireBinding(ctx, observation.id, observation.binding)
	if err != nil {
		return nil, err
	}
	borrowed, err := borrowInstalled(ctx, managed)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	current := !c.closed && c.sessions[session] == owned && owned.close == nil && managed.session == session && managed.instanceID == observation.instanceID
	c.mu.Unlock()
	if !current {
		return nil, errors.Join(ErrUnavailable, borrowed.Close())
	}
	return borrowed, nil
}

type borrowContextKey struct{}

// BorrowActivation restores only the original approved graph. The catalog's
// current role choices cannot retarget a durable command or waiting Run.
func (p *Pool) BorrowActivation(ctx context.Context, id mcpserver.ID, activation plugin.Activation) (*BorrowedSession, error) {
	if p.installed == nil {
		return nil, ErrUnavailable
	}
	p.inner.mu.Lock()
	closed := p.inner.closed
	p.inner.mu.Unlock()
	if closed {
		return nil, ErrUnavailable
	}
	binding, err := plugins.DecodeActivation(activation)
	if err != nil {
		return nil, err
	}
	managed, err := p.installed.acquireBinding(ctx, id, binding)
	if err != nil {
		return nil, err
	}
	borrowed, err := borrowInstalled(ctx, managed)
	if err != nil {
		return nil, err
	}
	p.inner.mu.Lock()
	closed = p.inner.closed
	p.inner.mu.Unlock()
	if closed {
		return nil, errors.Join(ErrUnavailable, borrowed.Close())
	}
	return borrowed, nil
}

func borrowInstalled(ctx context.Context, managed *installedSession) (*BorrowedSession, error) {
	work, err := managed.use.Lifetime().Enter(ctx)
	if err != nil {
		return nil, errors.Join(err, managed.release(context.WithoutCancel(ctx)))
	}
	borrowed := &BorrowedSession{ctx: work.Context(), session: managed.session, work: work, managed: managed,
		release: func() error { return managed.release(context.WithoutCancel(ctx)) },
	}
	borrowed.ctx = context.WithValue(borrowed.ctx, borrowContextKey{}, borrowed)
	return borrowed, nil
}

func (b *BorrowedSession) Activation() (plugin.Activation, error) {
	if b.managed == nil || b.ctx.Err() != nil {
		return plugin.Activation{}, ErrUnavailable
	}
	return b.managed.activation()
}

// SourceFingerprint describes the exact server realized during this Use's
// setup. Actor authority names its installation grant; it does not include
// the transport and executable configuration captured by a tool manifest.
func (b *BorrowedSession) SourceFingerprint() (identity.Digest, error) {
	if b.managed == nil || b.ctx.Err() != nil {
		return identity.Digest{}, ErrUnavailable
	}
	return b.managed.projection.fingerprint, nil
}
