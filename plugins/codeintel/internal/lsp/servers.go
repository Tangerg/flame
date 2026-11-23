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

	"github.com/samber/lo"
)

var ErrNoServer = errors.New("lsp: no language server for target")

var errClosed = errors.New("lsp: servers closed")

const diagnosticsSettle = 3 * time.Second

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

type clientSlot struct {
	done       chan struct{}
	cancel     context.CancelFunc
	client     *client
	err        error
	cleanupErr error
}

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

func (s *Servers) clientForFile(ctx context.Context, root, abs string) (*client, error) {
	spec, ok := s.table.forFile(abs)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoServer, filepath.Ext(abs))
	}
	return s.clientFor(ctx, root, spec)
}

// The launch is component-owned so canceling the first request cannot tear down a
// server another caller is waiting for.
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
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
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
	err := errors.Join(ctx.Err(), context.Cause(ctx))
	if err == nil {
		c, err = runClientLaunch(ctx, launch, spec, root)
	}
	if err == nil && c == nil {
		err = fmt.Errorf("lsp: start %s returned a nil client", spec.Name)
	}
	if err != nil && c != nil {
		err = errors.Join(err, c.close())
		if c.retire() == nil {
			c = nil
		}
	}

	s.mu.Lock()
	if !s.closed {
		pending.client = c
		pending.err = err
		if err != nil && c == nil && s.slots[key] == pending {
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
	select {
	case <-pending.done:
	default:
		select {
		case <-pending.done:
		case <-ctx.Done():
			return nil, errors.Join(ctx.Err(), context.Cause(ctx))
		}
	}
	if pending.err != nil {
		return nil, pending.err
	}
	return pending.client, nil
}

func resolve(root, file string) string {
	if filepath.IsAbs(file) {
		return filepath.Clean(file)
	}
	return filepath.Join(root, file)
}

func (s *Servers) Definition(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.definition(ctx, abs, pos)
}

func (s *Servers) References(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.references(ctx, abs, pos)
}

func (s *Servers) Implementation(ctx context.Context, root, file string, pos Position) ([]Location, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.implementation(ctx, abs, pos)
}

func (s *Servers) IncomingCalls(ctx context.Context, root, file string, pos Position) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.callHierarchy(ctx, abs, pos, (*client).incomingCalls)
}

func (s *Servers) OutgoingCalls(ctx context.Context, root, file string, pos Position) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.callHierarchy(ctx, abs, pos, (*client).outgoingCalls)
}

func (s *Servers) Hover(ctx context.Context, root, file string, pos Position) (string, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return "", err
	}
	return c.hover(ctx, abs, pos)
}

func (s *Servers) DocumentSymbols(ctx context.Context, root, file string) ([]Symbol, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.documentSymbols(ctx, abs)
}

func (s *Servers) WorkspaceSymbols(ctx context.Context, root, query string) ([]Symbol, error) {
	specs := s.table.forRoot(root)
	if len(specs) == 0 {
		return nil, ErrNoServer
	}
	var out []Symbol
	var errs []error
	for _, spec := range specs {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
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
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, errors.Join(append(errs, err)...)
	}
	return out, errors.Join(errs...)
}

func (s *Servers) Diagnostics(ctx context.Context, root, file string) ([]Diagnostic, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.diagnostics(ctx, abs, diagnosticsSettle)
}

func (s *Servers) InspectDocument(ctx context.Context, root, file string, content []byte) ([]Diagnostic, error) {
	abs := resolve(root, file)
	c, err := s.clientForFile(ctx, root, abs)
	if err != nil {
		return nil, err
	}
	return c.diagnosticsContent(ctx, abs, content, diagnosticsSettle)
}

func (s *Servers) Supported(file string) bool {
	_, ok := s.table.forFile(file)
	return ok
}

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
