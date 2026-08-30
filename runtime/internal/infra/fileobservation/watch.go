// Package fileobservation observes a bounded set of exact filesystem paths.
// It owns notification mechanics, path identity, debouncing, and goroutine
// lifetime; callers retain the meaning of each path through an opaque key.
package fileobservation

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/Tangerg/flame/runtime/internal/infra/pathidentity"
)

const debounce = 100 * time.Millisecond
const retryDelay = 500 * time.Millisecond

// Target is one exact path and its caller-owned classification key. Boundary,
// when non-empty, prevents a symlink target outside that physical root from
// being read or watched. This mirrors filesystem consumers that confine aliases
// to a selected scope.
type Target struct {
	Key      string
	Path     string
	Boundary string
}

// Observation owns one live exact-path observation and can accept selected
// identities as a new baseline when the caller has already published their
// semantic change through another authoritative path.
type Observation interface {
	io.Closer
	Accept(keys, identities []string) error
}

// Watch observes targets and calls notify with the distinct keys whose
// externally visible filesystem state changed. Missing targets are supported:
// their nearest existing ancestor is watched until the complete parent path is
// created. Close joins the observer before returning.
func Watch(targets []Target, notify func([]string)) (Observation, error) {
	canonical, err := canonicalTargets(targets)
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return nopWatch{}, nil
	}
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("observe files: create watcher: %w", err)
	}
	w := &watch{
		fsw: fsw, targets: canonical, notify: notify,
		fingerprints: make([]fingerprint, len(canonical)),
		watched:      make(map[string]struct{}),
		done:         make(chan struct{}),
		exited:       make(chan struct{}),
	}
	if err := w.reconcile(true, acceptance{}); err != nil {
		return nil, closeFailed(fsw, err)
	}
	go w.run()
	return w, nil
}

type target struct {
	key              string
	path             string
	physicalBoundary string
}

func canonicalTargets(targets []Target) ([]target, error) {
	out := make([]target, 0, len(targets))
	type targetKey struct {
		name string
		path string
	}
	seen := make(map[targetKey]struct{}, len(targets))
	for index, candidate := range targets {
		if candidate.Key == "" {
			return nil, fmt.Errorf("observe files: target %d key is required", index)
		}
		if candidate.Path == "" || !filepath.IsAbs(candidate.Path) {
			return nil, fmt.Errorf("observe files: target %d path must be absolute", index)
		}
		path := filepath.Clean(candidate.Path)
		identity := targetKey{name: candidate.Key, path: path}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		var boundary string
		if candidate.Boundary != "" {
			if !filepath.IsAbs(candidate.Boundary) {
				return nil, fmt.Errorf("observe files: target %d boundary must be absolute", index)
			}
			resolved, err := pathidentity.Resolve("", candidate.Boundary)
			if err != nil {
				return nil, fmt.Errorf("observe files: resolve target %d boundary: %w", index, err)
			}
			boundary = resolved
		}
		out = append(out, target{key: candidate.Key, path: path, physicalBoundary: boundary})
	}
	return out, nil
}

type watch struct {
	fsw     *fsnotify.Watcher
	targets []target
	notify  func([]string)

	fingerprints []fingerprint
	watched      map[string]struct{}
	done         chan struct{}
	exited       chan struct{}
	closeOnce    sync.Once
	stateMu      sync.Mutex
	closed       bool
}

func (w *watch) run() {
	defer close(w.exited)
	timer := time.NewTimer(debounce)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	armed := false
	armAfter := func(delay time.Duration) {
		if armed {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timer.Reset(delay)
		armed = true
	}
	arm := func() { armAfter(debounce) }
	for {
		select {
		case <-w.done:
			return
		case _, ok := <-w.fsw.Events:
			if !ok {
				return
			}
			arm()
		case _, ok := <-w.fsw.Errors:
			if !ok {
				return
			}
			// Overflow and transient backend errors mean the exact paths must be
			// sampled again; their fingerprints remain the source of truth.
			arm()
		case <-timer.C:
			armed = false
			if err := w.reconcile(false, acceptance{}); err != nil {
				// A path may disappear while its parent watches are being rebuilt,
				// and that rename may have produced the last backend event. Retry the
				// exact fingerprint instead of silently losing the transition.
				armAfter(retryDelay)
				continue
			}
		}
	}
}

type acceptance struct {
	keys       map[string]bool
	identities map[string]bool
}

func (a acceptance) matches(candidate target, physical string) bool {
	if !a.keys[candidate.key] {
		return false
	}
	return a.identities[candidate.path] || (physical != "" && a.identities[physical])
}

func (w *watch) reconcile(initial bool, accepted acceptance) error {
	w.stateMu.Lock()
	if w.closed {
		w.stateMu.Unlock()
		return nil
	}
	directories := make(map[string]struct{})
	next := make([]fingerprint, len(w.targets))
	changedKeys := make([]string, 0, len(w.targets))
	accepting := len(accepted.keys) > 0 && len(accepted.identities) > 0
	for index, candidate := range w.targets {
		state, physical, err := fingerprintOf(candidate)
		if err != nil {
			w.stateMu.Unlock()
			return err
		}
		matchesAccepted := accepting && accepted.matches(candidate, physical)
		switch {
		case initial:
			next[index] = state
		case matchesAccepted:
			next[index] = state
		case accepting:
			next[index] = w.fingerprints[index]
		default:
			next[index] = state
			if state != w.fingerprints[index] && !slices.Contains(changedKeys, candidate.key) {
				changedKeys = append(changedKeys, candidate.key)
			}
		}
		for _, path := range []string{candidate.path, physical} {
			if path == "" {
				continue
			}
			directory, err := nearestExistingDirectory(filepath.Dir(path))
			if err != nil {
				w.stateMu.Unlock()
				return err
			}
			directories[directory] = struct{}{}
		}
	}
	if err := w.replaceDirectories(directories); err != nil {
		w.stateMu.Unlock()
		return err
	}
	w.fingerprints = next
	w.stateMu.Unlock()
	if !accepting && len(changedKeys) > 0 && w.notify != nil {
		w.notify(changedKeys)
	}
	return nil
}

// Accept records selected exact paths' current filesystem state without
// emitting a callback. Other paths retain their previous baseline, so an
// unrelated concurrent edit is still reported by its queued filesystem event.
func (w *watch) Accept(keys, identities []string) error {
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
	return w.reconcile(false, accepted)
}

func fingerprintOf(candidate target) (fingerprint, string, error) {
	encoder := newFingerprintEncoder()
	encoder.field(fingerprintFieldLogicalPath, candidate.path)
	info, err := os.Lstat(candidate.path)
	if errors.Is(err, os.ErrNotExist) {
		encoder.state(fingerprintStateMissing)
		return encoder.sum(), "", nil
	}
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe files: inspect %q: %w", candidate.path, err)
	}
	encoder.fileInfo(fingerprintFieldLogicalInfo, info)
	if info.Mode()&os.ModeSymlink != 0 {
		destination, readErr := os.Readlink(candidate.path)
		if readErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe files: read symlink %q: %w", candidate.path, readErr)
		}
		encoder.field(fingerprintFieldLinkTarget, destination)
	}
	physical, err := pathidentity.Resolve("", candidate.path)
	if err != nil {
		encoder.state(fingerprintStateUnresolved)
		encoder.field(fingerprintFieldError, err.Error())
		return encoder.sum(), "", nil
	}
	if candidate.physicalBoundary != "" {
		inside, containsErr := pathidentity.Contains(candidate.physicalBoundary, physical)
		if containsErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe files: confine %q: %w", candidate.path, containsErr)
		}
		if !inside {
			encoder.state(fingerprintStateOutsideBoundary)
			encoder.field(fingerprintFieldPhysicalPath, physical)
			return encoder.sum(), "", nil
		}
	}
	encoder.field(fingerprintFieldPhysicalPath, physical)
	physicalInfo, err := os.Stat(physical)
	if errors.Is(err, os.ErrNotExist) {
		encoder.state(fingerprintStateMissingTarget)
		return encoder.sum(), physical, nil
	}
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe files: inspect target %q: %w", physical, err)
	}
	encoder.fileInfo(fingerprintFieldPhysicalInfo, physicalInfo)
	if physicalInfo.Mode().IsRegular() {
		file, openErr := os.Open(physical)
		if openErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe files: open %q: %w", physical, openErr)
		}
		copyErr := encoder.content(file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe files: read %q: %w", physical, errors.Join(copyErr, closeErr))
		}
	}
	return encoder.sum(), physical, nil
}

func nearestExistingDirectory(path string) (string, error) {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Stat(current)
		switch {
		case err == nil && info.IsDir():
			return current, nil
		case err == nil:
			return "", fmt.Errorf("observe files: ancestor %q is not a directory", current)
		case !errors.Is(err, os.ErrNotExist):
			return "", fmt.Errorf("observe files: inspect ancestor %q: %w", current, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("observe files: no existing ancestor for %q", path)
		}
	}
}

func (w *watch) replaceDirectories(next map[string]struct{}) error {
	for directory := range next {
		if _, present := w.watched[directory]; present {
			continue
		}
		if err := w.fsw.Add(directory); err != nil {
			return fmt.Errorf("observe files: watch directory %q: %w", directory, err)
		}
		w.watched[directory] = struct{}{}
	}
	for directory := range w.watched {
		if _, keep := next[directory]; keep {
			continue
		}
		if err := w.fsw.Remove(directory); err != nil && !errors.Is(err, fsnotify.ErrNonExistentWatch) {
			return fmt.Errorf("observe files: unwatch directory %q: %w", directory, err)
		}
		delete(w.watched, directory)
	}
	return nil
}

func (w *watch) Close() error {
	w.closeOnce.Do(func() {
		w.stateMu.Lock()
		w.closed = true
		w.stateMu.Unlock()
		close(w.done)
		<-w.exited
		_ = w.fsw.Close()
	})
	return nil
}

func closeFailed(watcher *fsnotify.Watcher, cause error) error {
	if err := watcher.Close(); err != nil {
		return errors.Join(cause, fmt.Errorf("observe files: close failed watcher: %w", err))
	}
	return cause
}

type nopWatch struct{}

func (nopWatch) Close() error                    { return nil }
func (nopWatch) Accept([]string, []string) error { return nil }
