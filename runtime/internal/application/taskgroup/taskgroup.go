package taskgroup

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/completion"
	"github.com/Tangerg/flame/runtime/internal/telemetry"
)

type Group struct {
	mu      sync.Mutex
	phase   groupPhase
	tasks   map[*registeredTask]struct{}
	idle    chan struct{}
	allDone chan struct{}
}

type groupPhase uint8

const (
	groupOpen groupPhase = iota
	groupClosed
	groupFinished
)

type registeredTask struct {
	cancel context.CancelFunc
}

func (g *Group) Start(parent context.Context, task func(context.Context)) bool {
	return g.spawn(parent, task, g.Attach)
}

func (g *Group) StartLinked(parent context.Context, task func(context.Context)) bool {
	return g.spawn(parent, task, g.AttachLinked)
}

func (g *Group) spawn(
	parent context.Context,
	task func(context.Context),
	attach func(context.Context) (context.Context, func(), bool),
) bool {
	if task == nil {
		return false
	}
	ctx, release, ok := attach(parent)
	if !ok {
		return false
	}
	go func() {
		defer release()
		detach(ctx, task)
	}()
	return true
}

func detach(ctx context.Context, task func(context.Context)) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.ErrorContext(ctx, "taskgroup: detached task panicked",
				telemetry.Panic(recovered))
		}
	}()
	task(ctx)
}

func (g *Group) Attach(parent context.Context) (ctx context.Context, release func(), ok bool) {
	return g.attach(context.WithoutCancel(parent))
}

func (g *Group) AttachLinked(parent context.Context) (ctx context.Context, release func(), ok bool) {
	return g.attach(parent)
}

func (g *Group) attach(parent context.Context) (ctx context.Context, release func(), ok bool) {
	ctx, cancel := context.WithCancel(parent)

	g.mu.Lock()
	if g.phase != groupOpen {
		g.mu.Unlock()
		cancel()
		return nil, nil, false
	}
	if g.tasks == nil {
		g.tasks = map[*registeredTask]struct{}{}
	}
	if len(g.tasks) == 0 {
		g.idle = make(chan struct{})
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
	if len(g.tasks) == 0 && g.idle != nil {
		close(g.idle)
		g.idle = nil
	}
	g.finishCloseLocked()
	g.mu.Unlock()
}

func (g *Group) Drain(ctx context.Context) error {
	if ctx == nil {
		return errors.New("taskgroup: drain context is required")
	}
	g.mu.Lock()
	if len(g.tasks) == 0 {
		g.mu.Unlock()
		return nil
	}
	idle := g.idle
	g.mu.Unlock()
	return completion.Wait(ctx, idle)
}

func (g *Group) Cancel() {
	g.mu.Lock()
	if g.phase == groupOpen {
		g.phase = groupClosed
	}
	tasks := slices.Collect(maps.Keys(g.tasks))
	g.finishCloseLocked()
	g.mu.Unlock()

	for _, registered := range tasks {
		registered.cancel()
	}
}

func (g *Group) Closed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.phase != groupOpen
}

func (g *Group) Wait(ctx context.Context) error {
	if ctx == nil {
		return errors.New("taskgroup: wait context is required")
	}
	g.mu.Lock()
	done := g.doneLocked()
	g.mu.Unlock()
	return completion.Wait(ctx, done)
}

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
	if g.phase != groupClosed || len(g.tasks) != 0 {
		return
	}
	if g.allDone == nil {
		g.allDone = make(chan struct{})
	}
	close(g.allDone)
	g.phase = groupFinished
}
