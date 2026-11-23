package delivery

import (
	"context"
	"errors"
	"iter"
	"sync"
)

type invocationGroup struct {
	lifetime context.Context

	mu     sync.Mutex
	phase  invocationPhase
	active map[*invocation]struct{}
	done   chan struct{}
}

type invocationPhase uint8

const (
	invocationsAdmitting invocationPhase = iota
	invocationsStopping
	invocationsFinished
)

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
	if i.phase != invocationsAdmitting || i.lifetime.Err() != nil {
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

func (i *invocationGroup) BeginShutdown() {
	i.mu.Lock()
	if i.phase == invocationsAdmitting {
		i.phase = invocationsStopping
	}
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
	if i.phase != invocationsStopping || len(i.active) != 0 {
		return
	}
	close(i.done)
	i.phase = invocationsFinished
}

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
