package mcp

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
)

// ErrSourceWithdrawn marks a configuration source that no longer permits a
// connection. Only the current attempt may remove its live entry.
var ErrSourceWithdrawn = errors.New("mcp: connection source withdrawn")

// Attempt owns connection admission before configuration or credential I/O.
// Run releases the admission gate and joins the result; Close cancels it.
// Shutdown also cancels and joins attempts whose gate was never released.
type Attempt struct {
	owner     *Connections
	target    *server
	ctx       context.Context
	cancel    context.CancelFunc
	resolve   func(context.Context, mcpserver.ID) (*Launch, error)
	authorize bool
	withdrawn *sessionCloseAttempt
	start     chan struct{}
	done      chan struct{}
	once      sync.Once
	err       error
}

func (c *Connections) BeginConnect(ctx context.Context, name mcpserver.ID, resolve func(context.Context, mcpserver.ID) (*Launch, error)) (*Attempt, error) {
	return c.begin(ctx, name, resolve, false)
}

func (c *Connections) BeginAuthorize(ctx context.Context, name mcpserver.ID, resolve func(context.Context, mcpserver.ID) (*Launch, error)) (*Attempt, error) {
	return c.begin(ctx, name, resolve, true)
}

func (c *Connections) begin(ctx context.Context, name mcpserver.ID, resolve func(context.Context, mcpserver.ID) (*Launch, error), authorize bool) (*Attempt, error) {
	if ctx == nil || resolve == nil {
		return nil, errors.New("mcp: attempt context and configuration source are required")
	}
	if err := name.Validate(); err != nil {
		return nil, fmt.Errorf("mcp: connection server: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil, ErrConnectionsClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	target := c.find(name)
	if target == nil {
		target = &server{id: name}
		c.servers = append(c.servers, target)
	}
	if target.attempt != nil {
		target.attempt.cancel()
	}
	ctx, cancel := context.WithCancel(ctx)
	attempt := &Attempt{
		owner: c, target: target, ctx: ctx, cancel: cancel, resolve: resolve,
		authorize: authorize, withdrawn: c.retireSessionLocked(target.session),
		start: make(chan struct{}), done: make(chan struct{}),
	}
	target.attempt = attempt
	target.settle(nil, nil, mcpserver.ConnectionConnecting, "")
	c.publishToolsLocked()
	c.attempts.Add(1)
	go attempt.run()
	return attempt, nil
}

func (a *Attempt) Run() error {
	a.once.Do(func() { close(a.start) })
	<-a.done
	return a.err
}

func (a *Attempt) Close() { a.cancel() }

func (a *Attempt) run() {
	defer a.owner.attempts.Done()
	defer close(a.done)
	defer a.cancel()
	a.err = a.execute()
	c := a.owner
	c.mu.Lock()
	if c.currentAttempt(a) {
		if errors.Is(a.err, ErrSourceWithdrawn) {
			// Detach by attempt identity; a stale source read cannot remove its successor.
			c.removeLocked(a.target)
		} else {
			a.target.settle(nil, nil, mcpserver.ConnectionFailed, mcpserver.FailureConfiguration)
			a.target.attempt = nil
		}
	}
	c.mu.Unlock()
}

// Attempts execute detached external code. A panic must still settle the
// attempt and release shutdown waiters, as an ordinary connection failure does.
func (a *Attempt) execute() (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mcp: connection attempt panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	select {
	case <-a.ctx.Done():
		return context.Cause(a.ctx)
	case <-a.start:
		return a.connect()
	}
}

func (a *Attempt) connect() (err error) {
	c := a.owner
	var closeErr error
	if a.withdrawn != nil {
		select {
		case <-a.withdrawn.done:
			closeErr = a.withdrawn.err
			c.mu.Lock()
			delete(c.retirements, a.withdrawn)
			c.mu.Unlock()
		case <-a.ctx.Done():
			return context.Cause(a.ctx)
		}
	}
	if err := a.ctx.Err(); err != nil {
		return errors.Join(closeErr, err)
	}
	input, err := a.resolve(a.ctx, a.target.name())
	if err != nil {
		return errors.Join(closeErr, err, context.Cause(a.ctx), input.Close())
	}
	prepared, err := input.take()
	if err != nil {
		return errors.Join(closeErr, err)
	}
	defer func() { err = errors.Join(closeErr, err, prepared.close()) }()
	if prepared.config.ID() != a.target.name() {
		return errors.New("mcp: connection configuration source changed identity")
	}
	if a.authorize {
		return c.authorize(a, prepared)
	}
	return c.configure(a, prepared)
}

// currentAttempt is called with c.mu held.
func (c *Connections) currentAttempt(attempt *Attempt) bool {
	return c.find(attempt.target.name()) == attempt.target && attempt.target.attempt == attempt
}

func (c *Connections) failAttempt(attempt *Attempt, failure mcpserver.ConnectionFailure) {
	c.mu.Lock()
	if c.currentAttempt(attempt) {
		attempt.target.settle(nil, nil, mcpserver.ConnectionFailed, failure)
		attempt.target.attempt = nil
	}
	c.mu.Unlock()
}
