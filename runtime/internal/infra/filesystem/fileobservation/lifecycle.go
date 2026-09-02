package fileobservation

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const debounce = 100 * time.Millisecond
const retryDelay = 500 * time.Millisecond

type acceptance struct {
	keys       map[string]bool
	identities map[string]bool
}

// observerLifecycle owns the one fsnotify resource and goroutine lifecycle
// shared by exact-file and bounded child-file reconciliation policies.
type observerLifecycle struct {
	label     string
	fsw       *fsnotify.Watcher
	watched   map[string]struct{}
	done      chan struct{}
	exited    chan struct{}
	closeOnce sync.Once
	stateMu   sync.Mutex
	closed    bool
	reconcile func(acceptance) error
}

func newObserverLifecycle(label string) (*observerLifecycle, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("%s: create watcher: %w", label, err)
	}
	return &observerLifecycle{
		label:   label,
		fsw:     fsw,
		watched: make(map[string]struct{}),
		done:    make(chan struct{}),
		exited:  make(chan struct{}),
	}, nil
}

func (o *observerLifecycle) start(reconcile func(acceptance) error) {
	o.reconcile = reconcile
	go o.run()
}

func (o *observerLifecycle) run() {
	defer close(o.exited)
	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	armed := false
	armAfter := func(delay time.Duration) {
		if armed && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(delay)
		armed = true
	}
	for {
		select {
		case <-o.done:
			return
		case _, ok := <-o.fsw.Events:
			if !ok {
				return
			}
			armAfter(debounce)
		case _, ok := <-o.fsw.Errors:
			if !ok {
				return
			}
			// Backend errors invalidate only the wake-up hint. Reconciliation
			// resamples the content-derived state that owns the observable fact.
			armAfter(debounce)
		case <-timer.C:
			armed = false
			if err := o.reconcile(acceptance{}); err != nil {
				// A rename may have produced the final backend event before path
				// watches were rebuilt, so retry without waiting for another hint.
				armAfter(retryDelay)
			}
		}
	}
}

func (o *observerLifecycle) Accept(keys, identities []string) error {
	accepted := acceptance{
		keys:       make(map[string]bool, len(keys)),
		identities: make(map[string]bool, len(identities)),
	}
	for _, key := range keys {
		if key != "" {
			accepted.keys[key] = true
		}
	}
	for _, identity := range identities {
		if filepath.IsAbs(identity) {
			accepted.identities[filepath.Clean(identity)] = true
		}
	}
	if len(accepted.keys) == 0 || len(accepted.identities) == 0 {
		return nil
	}
	return o.reconcile(accepted)
}

func (o *observerLifecycle) replaceDirectories(next map[string]struct{}) error {
	for directory := range next {
		if _, present := o.watched[directory]; present {
			continue
		}
		if err := o.fsw.Add(directory); err != nil {
			return fmt.Errorf("%s: watch directory %q: %w", o.label, directory, err)
		}
		o.watched[directory] = struct{}{}
	}
	for directory := range o.watched {
		if _, keep := next[directory]; keep {
			continue
		}
		if err := o.fsw.Remove(directory); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			return fmt.Errorf("%s: unwatch directory %q: %w", o.label, directory, err)
		}
		delete(o.watched, directory)
	}
	return nil
}

func (o *observerLifecycle) abort(cause error) error {
	if err := o.fsw.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("%s: close failed watcher: %w", o.label, err))
	}
	return cause
}

func (o *observerLifecycle) Close() error {
	o.closeOnce.Do(func() {
		o.stateMu.Lock()
		o.closed = true
		o.stateMu.Unlock()
		close(o.done)
		<-o.exited
		_ = o.fsw.Close()
	})
	return nil
}
