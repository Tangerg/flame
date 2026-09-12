package toolset

import (
	"context"
	"sync"

	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/scope/core/chat"
)

// pathLocker serializes file tool calls that target the same resolved path.
// Separate runs can execute concurrently, so two mutations must not
// interleave, and a tracked read must stamp the exact state it read.
// Keyed by resolved abs path and ref-counted so the map doesn't grow unbounded;
// glob / grep / LSP / MCP tools are unaffected. The runtime resolver owns one
// locker across all of its per-Run tool builds, so separate Runs cannot both
// cross a stale-read check before either mutation lands.
type pathLocker struct {
	mu    sync.Mutex
	locks map[string]*pathLock
}

type pathLock struct {
	ready chan struct{}
	refs  int
}

func newPathLocker() *pathLocker { return &pathLocker{locks: make(map[string]*pathLock)} }

// acquire takes the per-path lock and returns a release closure. Calls for
// distinct paths run concurrently; calls for the same path serialize. Waiting
// is context-aware so a canceled Run is never pinned behind another tool call.
func (p *pathLocker) acquire(ctx context.Context, path string) (func(), error) {
	p.mu.Lock()
	l := p.locks[path]
	if l == nil {
		l = &pathLock{ready: make(chan struct{}, 1)}
		l.ready <- struct{}{}
		p.locks[path] = l
	}
	l.refs++
	p.mu.Unlock()

	if err := ctx.Err(); err != nil {
		p.releaseRef(path, l)
		return nil, err
	}
	select {
	case <-ctx.Done():
		p.releaseRef(path, l)
		return nil, ctx.Err()
	case <-l.ready:
	}
	if err := ctx.Err(); err != nil {
		l.ready <- struct{}{}
		p.releaseRef(path, l)
		return nil, err
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			l.ready <- struct{}{}
			p.releaseRef(path, l)
		})
	}, nil
}

func (p *pathLocker) releaseRef(path string, l *pathLock) {
	p.mu.Lock()
	if l.refs--; l.refs == 0 {
		delete(p.locks, path)
	}
	p.mu.Unlock()
}

// withPathLock wraps a file tool so concurrent calls targeting the same resolved
// path run one-at-a-time (see [pathLocker]). For mutations it is applied inside
// the path guard but outside the staleness / diagnostics / mutation chain. For
// reads it encloses both the filesystem read and tracker stamp. Scheduling
// remains the inner Tool's policy; this lock coordinates physical paths across
// Runs, including relative, absolute, and symlink aliases.
func withPathLock(inner toolcontract.Tool, locker *pathLocker, cwd string) toolcontract.Tool {
	return &pathLocked{inner: inner, locker: locker, cwd: cwd}
}

// pathLocked keeps the filesystem operation and its tracking under one lock.
type pathLocked struct {
	inner  toolcontract.Tool
	locker *pathLocker
	cwd    string
}

func (p *pathLocked) Definition() chat.ToolDefinition { return p.inner.Definition() }

// Unwrap preserves the inner Tool's scheduling and mutation declarations.
func (p *pathLocked) Unwrap() toolcontract.Tool { return p.inner }

func (p *pathLocked) Call(ctx context.Context, invocation toolcontract.Invocation) (chat.ToolOutput, error) {
	paths, err := resolvedMutationPaths(p.inner, invocation, p.cwd)
	if err != nil {
		return chat.ToolOutput{}, err
	}
	for _, path := range paths {
		release, err := p.locker.acquire(ctx, path)
		if err != nil {
			return chat.ToolOutput{}, err
		}
		defer release()
	}
	return p.inner.Call(ctx, invocation)
}
