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

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

// Target is one exact path and its caller-owned classification key. Boundary,
// when non-empty, prevents a symlink target outside that physical root from
// being read or watched. This mirrors filesystem consumers that confine aliases
// to a selected scope. MaxBytes is required and bounds content hashing; larger
// files remain observable through their metadata without being read.
type Target struct {
	Key      string
	Path     string
	Boundary string
	MaxBytes int64
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
// created. When provided, report receives the first background reconciliation
// error in each outage while retries continue. Close joins both callbacks before
// returning; callbacks must return promptly and must not close the observation.
func Watch(targets []Target, notify func([]string), report func(error)) (Observation, error) {
	canonical, err := canonicalTargets(targets)
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return nopWatch{}, nil
	}
	roots, lifecycle, err := openObservation("observe files", canonical, func(candidate target) string {
		return candidate.physicalBoundary
	}, report)
	if err != nil {
		return nil, err
	}
	w := &watch{
		observerLifecycle: lifecycle,
		targets:           canonical,
		notify:            notify,
		fingerprints:      make([]fingerprint, len(canonical)),
		roots:             roots,
	}
	if err := w.reconcile(true, acceptance{}); err != nil {
		return nil, errors.Join(lifecycle.abort(err), roots.Close())
	}
	lifecycle.start(func(accepted acceptance) error {
		return w.reconcile(false, accepted)
	})
	return w, nil
}

type target struct {
	key              string
	path             string
	physicalBoundary string
	maxBytes         int64
}

// openObservation opens the roots a canonical target set needs and the
// lifecycle that will drive it. Both watchers assemble this way, and both must
// close the roots when the lifecycle refuses to start — the one failure path
// that leaks a descriptor if it is written twice and one copy forgets.
func openObservation[T any](
	label string,
	canonical []T,
	boundary func(T) string,
	report func(error),
) (*observationRoots, *observerLifecycle, error) {
	boundaries := make([]string, len(canonical))
	for index, candidate := range canonical {
		boundaries[index] = boundary(candidate)
	}
	roots, err := openObservationRoots(boundaries)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", label, err)
	}
	lifecycle, err := newObserverLifecycle(label, report)
	if err != nil {
		return nil, nil, errors.Join(err, roots.Close())
	}
	return roots, lifecycle, nil
}

// canonicalTargetSet maps each candidate through one, dropping repeats while
// keeping first-seen order. Both watchers canonicalize this way; only what a
// target is differs.
func canonicalTargetSet[In any, Out comparable](targets []In, one func(int, In) (Out, error)) ([]Out, error) {
	out := make([]Out, 0, len(targets))
	seen := make(map[Out]struct{}, len(targets))
	for index, candidate := range targets {
		canonical, err := one(index, candidate)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[canonical]; duplicate {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
	}
	return out, nil
}

func canonicalTargets(targets []Target) ([]target, error) {
	return canonicalTargetSet(targets, canonicalTarget)
}

func canonicalTarget(index int, candidate Target) (target, error) {
	if candidate.Key == "" {
		return target{}, fmt.Errorf("observe files: target %d key is required", index)
	}
	if candidate.Path == "" || !filepath.IsAbs(candidate.Path) {
		return target{}, fmt.Errorf("observe files: target %d path must be absolute", index)
	}
	if candidate.MaxBytes <= 0 {
		return target{}, fmt.Errorf("observe files: target %d byte limit must be positive", index)
	}
	boundary, err := resolveTargetBoundary(index, candidate.Boundary)
	if err != nil {
		return target{}, err
	}
	return target{
		key: candidate.Key, path: filepath.Clean(candidate.Path),
		physicalBoundary: boundary, maxBytes: candidate.MaxBytes,
	}, nil
}

func resolveTargetBoundary(index int, boundary string) (string, error) {
	if boundary == "" {
		return "", nil
	}
	if !filepath.IsAbs(boundary) {
		return "", fmt.Errorf("observe files: target %d boundary must be absolute", index)
	}
	resolved, err := pathidentity.Resolve("", boundary)
	if err != nil {
		return "", fmt.Errorf("observe files: resolve target %d boundary: %w", index, err)
	}
	return resolved, nil
}

type watch struct {
	*observerLifecycle
	targets []target
	notify  func([]string)
	roots   *observationRoots

	fingerprints []fingerprint
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
		observed, physical, err := fingerprintOf(candidate, w.roots)
		if err != nil {
			w.stateMu.Unlock()
			return err
		}
		match := noAcceptedWrite
		switch {
		case !accepting:
		case accepted.matches(candidate, physical):
			match = acceptedWriteHere
		default:
			match = acceptedWriteElsewhere
		}
		var changed bool
		next[index], changed = advanceFingerprint(
			initial, match, w.fingerprints[index], observed,
		)
		if changed && !slices.Contains(changedKeys, candidate.key) {
			changedKeys = append(changedKeys, candidate.key)
		}
		if err := collectParentDirectories(directories, candidate.path, physical); err != nil {
			w.stateMu.Unlock()
			return err
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

// advanceFingerprint is the exact-file baseline policy. Initial sampling and
// explicitly accepted identities advance immediately. During any acceptance
// batch, unrelated targets retain their prior baseline so the next ordinary
// resample still publishes their independently observed change.
// acceptanceMatch says how one target relates to a write the watcher accepted:
// nothing was accepted, something was and this target is not it, or this target
// is exactly it. A target can only be the accepted write when there is one, so
// the pair of booleans that used to say this could name a state that never
// happens.
type acceptanceMatch uint8

const (
	noAcceptedWrite acceptanceMatch = iota + 1
	acceptedWriteElsewhere
	acceptedWriteHere
)

// advanceFingerprint decides what one target's next fingerprint is and whether
// the difference is worth reporting. A first pass and the write we accepted are
// both adopted silently; while an accepted write is settling, every other
// target holds what it had.
func advanceFingerprint(
	initial bool,
	match acceptanceMatch,
	previous fingerprint,
	observed fingerprint,
) (fingerprint, bool) {
	switch {
	case initial || match == acceptedWriteHere:
		return observed, false
	case match == acceptedWriteElsewhere:
		return previous, false
	default:
		return observed, observed != previous
	}
}

func collectParentDirectories(directories map[string]struct{}, paths ...string) error {
	for _, path := range paths {
		if path == "" {
			continue
		}
		directory, err := nearestExistingDirectory(filepath.Dir(path))
		if err != nil {
			return err
		}
		directories[directory] = struct{}{}
	}
	return nil
}

func fingerprintOf(candidate target, roots *observationRoots) (fingerprint, string, error) {
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
	return fingerprintPhysicalTarget(encoder, candidate, physical, roots)
}

func fingerprintPhysicalTarget(
	encoder *fingerprintEncoder,
	candidate target,
	physical string,
	roots *observationRoots,
) (_ fingerprint, _ string, err error) {
	root, name, inside, err := roots.access(candidate.physicalBoundary, physical)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe files: confine %q: %w", candidate.path, err)
	}
	if !inside {
		encoder.state(fingerprintStateOutsideBoundary)
		encoder.field(fingerprintFieldPhysicalPath, physical)
		return encoder.sum(), "", nil
	}
	encoder.field(fingerprintFieldPhysicalPath, physical)
	var physicalInfo os.FileInfo
	if root != nil {
		physicalInfo, err = root.Stat(name)
	} else {
		physicalInfo, err = os.Stat(name)
	}
	if errors.Is(err, os.ErrNotExist) {
		encoder.state(fingerprintStateMissingTarget)
		return encoder.sum(), physical, nil
	}
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe files: inspect target %q: %w", physical, err)
	}
	if physicalInfo.Mode().IsRegular() {
		if physicalInfo.Size() > candidate.maxBytes {
			encoder.fileInfo(fingerprintFieldPhysicalInfo, physicalInfo)
			encoder.state(fingerprintStateTooLarge)
			return encoder.sum(), physical, nil
		}
		var file *os.File
		var opened os.FileInfo
		var openErr error
		if root != nil {
			file, opened, openErr = fileinput.OpenAtExpected(root, name, physicalInfo, candidate.maxBytes)
		} else {
			file, opened, openErr = fileinput.OpenExpected(name, physicalInfo, candidate.maxBytes)
		}
		if openErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe files: open %q: %w", physical, openErr)
		}
		encoder.fileInfo(fingerprintFieldPhysicalInfo, opened)
		copyErr := encoder.content(file, opened.Size())
		versionErr := verifyObservedFileVersion(root, name, file, opened)
		closeErr := file.Close()
		if copyErr != nil || versionErr != nil || closeErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe files: read %q: %w", physical, errors.Join(copyErr, versionErr, closeErr))
		}
	} else {
		encoder.fileInfo(fingerprintFieldPhysicalInfo, physicalInfo)
	}
	return encoder.sum(), physical, nil
}

func (w *watch) Close() error {
	return errors.Join(w.observerLifecycle.Close(), w.roots.Close())
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

type nopWatch struct{}

func (nopWatch) Close() error                    { return nil }
func (nopWatch) Accept([]string, []string) error { return nil }
