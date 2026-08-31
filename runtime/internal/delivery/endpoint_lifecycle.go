package delivery

import (
	"context"
	"errors"
	"iter"
	"sync"
)

// invocationGroup owns Endpoint admission and the exact lifetime of every
// accepted unary call or stream. It is delivery-local: application task groups
// continue to own request-detached product work, while this group prevents the
// process owner from closing their dependencies underneath an active binding
// call.
type invocationGroup struct {
	lifetime context.Context

	mu       sync.Mutex
	stopping bool
	finished bool
	active   map[*invocation]struct{}
	done     chan struct{}
}

// invocation is one accepted binding call and its cancellation capability.
// It is process-local, so its object identity is the registry key.
type invocation struct {
	cancel context.CancelFunc
}

func newInvocationGroup(lifetime context.Context) *invocationGroup {
	group := &invocationGroup{
		lifetime: lifetime,
		active:   make(map[*invocation]struct{}),
		done:     make(chan struct{}),
	}
	context.AfterFunc(lifetime, group.BeginShutdown)
	return group
}

func (i *invocationGroup) Attach(parent context.Context) (context.Context, func(), bool) {
	ctx, cancel := context.WithCancel(parent)
	i.mu.Lock()
	if i.stopping || i.lifetime.Err() != nil {
		i.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	registered := &invocation{cancel: cancel}
	i.active[registered] = struct{}{}
	i.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			registered.cancel()
			i.mu.Lock()
			delete(i.active, registered)
			i.finishShutdownLocked()
			i.mu.Unlock()
		})
	}
	return ctx, release, true
}

// BeginShutdown closes admission and broadcasts cancellation before waiting on
// any one operation. Calls already inside their source remain registered until
// they actually return.
func (i *invocationGroup) BeginShutdown() {
	i.mu.Lock()
	i.stopping = true
	invocations := make([]*invocation, 0, len(i.active))
	for registered := range i.active {
		invocations = append(invocations, registered)
	}
	i.finishShutdownLocked()
	i.mu.Unlock()

	for _, registered := range invocations {
		registered.cancel()
	}
}

func (i *invocationGroup) AwaitShutdown(ctx context.Context) error {
	if ctx == nil {
		return errors.New("operation: shutdown context is required")
	}
	select {
	case <-i.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (i *invocationGroup) finishShutdownLocked() {
	if !i.stopping || len(i.active) != 0 || i.finished {
		return
	}
	close(i.done)
	i.finished = true
}

// ownStream keeps one accepted operation registered until its source returns.
// If a caller never starts ranging, cancellation claims and ranges the source
// itself with a rejecting yield. That joins source-side teardown instead of
// merely assuming every context.AfterFunc callback has already completed.
func ownStream(
	ctx context.Context,
	events iter.Seq2[any, error],
	release func(),
) iter.Seq2[any, error] {
	var (
		mu      sync.Mutex
		claimed bool
	)
	finish := sync.OnceFunc(release)
	run := func(yield func(any, error) bool) {
		defer finish()
		events(yield)
	}

	stopAbandon := context.AfterFunc(ctx, func() {
		mu.Lock()
		if claimed {
			mu.Unlock()
			return
		}
		claimed = true
		mu.Unlock()
		run(func(any, error) bool { return false })
	})

	return func(yield func(any, error) bool) {
		mu.Lock()
		if claimed {
			mu.Unlock()
			return
		}
		claimed = true
		mu.Unlock()
		stopAbandon()
		run(yield)
	}
}
