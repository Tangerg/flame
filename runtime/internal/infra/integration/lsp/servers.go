// Package lsp drives language servers as child processes over the Language
// Server Protocol, exposing code intelligence (definition, references, hover,
// symbols, diagnostics) to the agent. It is platform-agnostic: the only OS
// dependency is the server binaries the user has installed — the protocol
// itself is plain JSON-RPC over the process's stdio.
//
// Servers is the entry point. It starts one language server per (workspace
// root, language) on first use and keeps it warm for the engine's lifetime.
// Adding a language is a one-line ServerSpec (see server.go); nothing here is
// Go-specific.
package lsp

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/samber/lo"
)

// ErrNoServer means no configured language server handles the target — an
// unsupported file extension, or a workspace with no matching root marker.
var ErrNoServer = errors.New("lsp: no language server for target")

// ErrClosed means the server set has been shut down.
var errClosed = errors.New("lsp: servers closed")

// diagnosticsSettle bounds how long a diagnostics query waits for the server
// to analyze a just-synced document before reporting a pending observation.
const diagnosticsSettle = 3 * time.Second

// Servers owns the live language-server connections for one engine. It is safe
// for concurrent use: many sessions share it, each scoped to its own workspace
// root via the root argument the tools pass.
type Servers struct {
	lifetime context.Context
	table    *serverTable
	launch   clientLauncher

	mu     sync.Mutex
	slots  map[serverClientKey]*clientSlot
	closed bool

	closeOnce sync.Once
	closeErr  error
}

type clientLauncher func(context.Context, ServerSpec, string) (*client, error)

// clientSlot is one connection generation for a workspace/server. Its pending
// boundary owns both retirement of the previous generation and initialization
// of its replacement. Callers may stop waiting without canceling that boundary.
type clientSlot struct {
	done       chan struct{}
	cancel     context.CancelFunc
	client     *client
	err        error
	cleanupErr error
}

// NewServers builds a server set over specs (see DefaultServers for the
// built-in table). It starts no processes — servers launch lazily on first use.
func NewServers(lifetime context.Context, specs []ServerSpec) (*Servers, error) {
	if lo.IsNil(lifetime) {
		return nil, errors.New("lsp: lifetime is required")
	}
	table, err := newServerTable(specs)
	if err != nil {
		return nil, err
	}
	return &Servers{
		lifetime: lifetime,
		table:    table,
		launch:   startClient,
		slots:    map[serverClientKey]*clientSlot{},
	}, nil
}

type serverClientKey struct {
	root string
	name string
}

func clientKey(root, name string) serverClientKey { return serverClientKey{root: root, name: name} }

func compareServerClientKeys(left, right serverClientKey) int {
	if compared := cmp.Compare(left.root, right.root); compared != 0 {
		return compared
	}
	return cmp.Compare(left.name, right.name)
}

// clientForFile resolves the server for abs's extension and returns its
// connection for root, starting it if needed.
func (s *Servers) clientForFile(ctx context.Context, root, abs string) (*client, error) {
	spec, ok := s.table.forFile(abs)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoServer, filepath.Ext(abs))
	}
	return s.clientFor(ctx, root, spec)
}

// clientFor returns the connection for (root, spec), starting it on first use.
// Concurrent callers for the same key share one launch; different keys still
// start independently. The launch is component-owned so cancellation of the
// first request only stops that caller waiting — it cannot tear down a server
// another concurrent caller is waiting for. Close cancels and joins every
// in-flight launch.
func (s *Servers) clientFor(ctx context.Context, root string, spec ServerSpec) (*client, error) {
	key := clientKey(root, spec.Name)

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, errClosed
	}
	var previous *client
	if slot, ok := s.slots[key]; ok {
		select {
		case <-slot.done:
			if slot.err != nil {
				s.mu.Unlock()
				return nil, slot.err
			}
			select {
			case <-slot.client.conn.DisconnectNotify():
				previous = slot.client
			default:
				s.mu.Unlock()
				return slot.client, nil
			}
		default:
			s.mu.Unlock()
			return awaitClientStart(ctx, slot)
		}
	}
	if err := cancellation.Err(ctx); err != nil {
		s.mu.Unlock()
		return nil, err
	}

	startCtx, cancel := context.WithCancel(s.lifetime)
	pending := &clientSlot{done: make(chan struct{}), cancel: cancel}
	s.slots[key] = pending
	launch := s.launch
	s.mu.Unlock()

	go s.launchClient(startCtx, key, root, spec, pending, previous, launch)
	return awaitClientStart(ctx, pending)
}

// runClientLaunch keeps a defect inside an external server's handshake from
// ending the process and stranding every caller waiting on this start. A launch
// that fails already lands in the pending start's error; a launch that panics is
// one of those.
func runClientLaunch(
	ctx context.Context,
	launch clientLauncher,
	spec ServerSpec,
	root string,
) (c *client, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			c = nil
			err = fmt.Errorf("lsp: start %s panicked: %v\n%s", spec.Name, recovered, debug.Stack())
		}
	}()
	return launch(ctx, spec, root)
}

func (s *Servers) launchClient(
	ctx context.Context,
	key serverClientKey,
	root string,
	spec ServerSpec,
	pending *clientSlot,
	previous *client,
	launch clientLauncher,
) {
	defer pending.cancel()
	if previous != nil {
		if err := previous.retire(); err != nil {
			s.mu.Lock()
			pending.client = previous
			pending.err = fmt.Errorf("lsp: retire %s before replacement: %w", spec.Name, err)
			close(pending.done)
			s.mu.Unlock()
			return
		}
	}
	var c *client
	err := cancellation.Err(ctx)
	if err == nil {
		c, err = runClientLaunch(ctx, launch, spec, root)
	}
	if err == nil && c == nil {
		err = fmt.Errorf("lsp: start %s returned a nil client", spec.Name)
	}
	if err != nil && c != nil {
		err = errors.Join(err, c.close())
		c = nil
	}

	s.mu.Lock()
	if !s.closed {
		pending.client = c
		pending.err = err
		if err != nil && s.slots[key] == pending {
			delete(s.slots, key)
		}
		close(pending.done)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()

	if c != nil {
		pending.cleanupErr = c.close()
	}
	pending.err = errClosed
	close(pending.done)
}

func awaitClientStart(ctx context.Context, pending *clientSlot) (*client, error) {
	// Prefer a completed launch over a racing caller cancellation.
	select {
	case <-pending.done:
		return pending.client, pending.err
	default:
	}
	select {
	case <-pending.done:
		return pending.client, pending.err
	case <-ctx.Done():
		return nil, cancellation.Err(ctx)
	}
}

// resolve makes file absolute against the workspace root.
func resolve(root, file string) string {
	if filepath.IsAbs(file) {
		return filepath.Clean(file)
	}
	return filepath.Join(root, file)
}

// Definition returns the declaration site(s) of the symbol at pos — 0-based,
// as on the wire. file may be absolute or relative to root.
func (s *Servers) Definition(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.definition(ctx, abs, pos)
}

// References returns the use sites of the symbol at pos, including its
// declaration.
func (s *Servers) References(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.references(ctx, abs, pos)
}

// Implementation returns the concrete implementation site(s) of the interface
// or abstract method at pos.
func (s *Servers) Implementation(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.implementation(ctx, abs, pos)
}

// IncomingCalls returns the callers of the function/method at pos (who calls
// it), as symbols. OutgoingCalls returns its callees (what it calls).
func (s *Servers) IncomingCalls(ctx context.Context, root, file string, pos Position) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.callHierarchy(ctx, abs, pos, false)
}

func (s *Servers) OutgoingCalls(ctx context.Context, root, file string, pos Position) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.callHierarchy(ctx, abs, pos, true)
}

// Hover returns the server's hover text (signature, doc) at pos, as plain text.
func (s *Servers) Hover(ctx context.Context, root, file string, pos Position) (string, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return "", err
	}
	return c.hover(ctx, abs, pos)
}

// DocumentSymbols returns the symbols declared in file.
func (s *Servers) DocumentSymbols(ctx context.Context, root, file string) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.documentSymbols(ctx, abs)
}

// WorkspaceSymbols searches the whole workspace for symbols matching query,
// across every language whose root marker is present. A failed language does
// not discard another language's successful result. Every failure is joined in
// configured-server order alongside any successful symbols. Returns
// ErrNoServer when no configured language applies to root.
func (s *Servers) WorkspaceSymbols(ctx context.Context, root, query string) ([]Symbol, error) {
	specs := s.table.forRoot(root)
	if len(specs) == 0 {
		return nil, ErrNoServer
	}
	var out []Symbol
	var errs []error
	for _, spec := range specs {
		if err := cancellation.Err(ctx); err != nil {
			return nil, errors.Join(append(errs, err)...)
		}
		c, err := s.clientFor(ctx, root, spec)
		if err != nil {
			errs = append(errs, fmt.Errorf("lsp: start %s for workspace symbols: %w", spec.Name, err))
			continue
		}
		syms, err := c.workspaceSymbols(ctx, query)
		if err != nil {
			errs = append(errs, fmt.Errorf("lsp: query workspace symbols from %s: %w", spec.Name, err))
			continue
		}
		out = append(out, syms...)
	}
	if err := cancellation.Err(ctx); err != nil {
		return nil, errors.Join(append(errs, err)...)
	}
	return out, errors.Join(errs...)
}

// Diagnostics returns the server's current problems for file, syncing it first
// and waiting briefly for a fresh analysis.
func (s *Servers) Diagnostics(ctx context.Context, root, file string) ([]Diagnostic, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.diagnostics(ctx, abs, diagnosticsSettle)
}

// Supported reports whether any configured server handles file's extension —
// lets callers skip LSP work (e.g. post-mutation diagnostics) for unsupported
// files without starting anything.
func (s *Servers) Supported(file string) bool {
	_, ok := s.table.forFile(file)
	return ok
}

// Close shuts every live server down. Safe to call multiple times.
func (s *Servers) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		slots := maps.Clone(s.slots)
		s.slots = nil
		s.mu.Unlock()

		keys := make([]serverClientKey, 0, len(slots))
		for key := range slots {
			keys = append(keys, key)
		}
		slices.SortFunc(keys, compareServerClientKeys)
		for _, key := range keys {
			slots[key].cancel()
		}
		var errs []error
		for _, key := range keys {
			slot := slots[key]
			<-slot.done
			if slot.client != nil {
				if err := slot.client.close(); err != nil {
					errs = append(errs, err)
				}
			}
			if slot.cleanupErr != nil {
				errs = append(errs, slot.cleanupErr)
			}
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}
