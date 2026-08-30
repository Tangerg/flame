// Package taskgroup owns cancelable, request-detached work launched by
// application components. It provides the lifecycle boundary a component uses
// to launch, cancel, and join its own background tasks without knowing what
// those tasks do.
package taskgroup

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/completion"
)

// Group starts request-detached tasks and cancels and joins them at Close.
// The zero value is ready to use. Start and Close are safe to call
// concurrently; once closed, a Group cannot be reused.
type Group struct {
	mu       sync.Mutex
	closed   bool
	finished bool
	tasks    map[*registeredTask]struct{}
	allDone  chan struct{}
}

// registeredTask is the identity and cancellation capability of one attached
// task. It never leaves the owning Group, so object identity is the exact
// registry key; a process-local numeric ID would only add a wraparound alias.
type registeredTask struct {
	cancel context.CancelFunc
}

// Start launches task with parent values but without parent cancellation. It
// returns false when task is nil or the group is already closed.
func (g *Group) Start(parent context.Context, task func(context.Context)) bool {
	if task == nil {
		return false
	}
	ctx, release, ok := g.Attach(parent)
	if !ok {
		return false
	}

	go func() {
		defer release()
		task(ctx)
	}()
	return true
}

// StartLinked launches task under a context canceled by either parent or Close.
// Use it when the caller has already derived a component-owned operation context
// whose supersession must interrupt the background task.
func (g *Group) StartLinked(parent context.Context, task func(context.Context)) bool {
	if task == nil {
		return false
	}
	ctx, release, ok := g.AttachLinked(parent)
	if !ok {
		return false
	}
	go func() {
		defer release()
		task(ctx)
	}()
	return true
}

// Attach registers caller-managed work with the group. The returned context
// preserves parent values, ignores parent cancellation, and is canceled by
// Close. The caller must release it when the work ends; release is idempotent.
func (g *Group) Attach(parent context.Context) (ctx context.Context, release func(), ok bool) {
	return g.attach(context.WithoutCancel(parent))
}

// AttachLinked registers caller-managed work whose context is canceled by
// either the parent or Close. Use it for inbound calls owned jointly by the
// caller and the component; background maintenance should use Attach instead.
func (g *Group) AttachLinked(parent context.Context) (ctx context.Context, release func(), ok bool) {
	return g.attach(parent)
}

func (g *Group) attach(parent context.Context) (ctx context.Context, release func(), ok bool) {
	ctx, cancel := context.WithCancel(parent)

	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	if g.tasks == nil {
		g.tasks = map[*registeredTask]struct{}{}
	}
	registered := &registeredTask{cancel: cancel}
	g.tasks[registered] = struct{}{}
	g.mu.Unlock()

	var once sync.Once
	release = func() {
		once.Do(func() { g.finish(registered) })
	}
	return ctx, release, true
}

func (g *Group) finish(registered *registeredTask) {
	registered.cancel()
	g.mu.Lock()
	delete(g.tasks, registered)
	g.finishCloseLocked()
	g.mu.Unlock()
}

// Cancel rejects new tasks and cancels every active task. It does not wait for
// them to return, allowing a process owner to stop every component before it
// starts joining any one component.
func (g *Group) Cancel() {
	g.mu.Lock()
	g.closed = true
	tasks := slices.Collect(maps.Keys(g.tasks))
	g.finishCloseLocked()
	g.mu.Unlock()

	for _, registered := range tasks {
		registered.cancel()
	}
}

// Wait joins all active tasks after [Cancel]. The caller owns the deadline, so
// a shutdown timeout becomes observable instead of silently leaking work.
func (g *Group) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("taskgroup: wait context is required")
	}
	g.mu.Lock()
	done := g.doneLocked()
	g.mu.Unlock()
	return completion.Wait(ctx, done)
}

// Close rejects new tasks, cancels active tasks, and waits for them to return.
// It is safe to call repeatedly.
func (g *Group) Close(ctx context.Context) error {
	g.Cancel()
	return g.Wait(ctx)
}

func (g *Group) doneLocked() chan struct{} {
	if g.allDone == nil {
		g.allDone = make(chan struct{})
	}
	g.finishCloseLocked()
	return g.allDone
}

func (g *Group) finishCloseLocked() {
	if !g.closed || len(g.tasks) != 0 || g.finished {
		return
	}
	if g.allDone == nil {
		g.allDone = make(chan struct{})
	}
	close(g.allDone)
	g.finished = true
}
