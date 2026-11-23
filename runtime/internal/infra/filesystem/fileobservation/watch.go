package fileobservation

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

type Target struct {
	Key        string
	Path       string
	Boundary   string
	MaxBytes   int64
	MaxEntries int
}

type Observation interface {
	io.Closer
	Accept(keys, identities []string) error
}

// Callbacks must return promptly and must not close the observation; Close joins
// both before returning.
func Watch(targets []Target, notify func([]string), report func(error)) (Observation, error) {
	if notify == nil {
		return nil, errors.New("observe files: change notifier is required")
	}
	canonical, err := canonicalTargets(targets)
	if err != nil {
		return nil, err
	}
	return startObservation("observe files", canonical, func(candidate target) string {
		return candidate.physicalBoundary
	}, report, func(lifecycle *observerLifecycle, roots *observationRoots) *watch {
		return &watch{
			observerLifecycle: lifecycle,
			targets:           canonical,
			notify:            notify,
			fingerprints:      make([]fingerprint, len(canonical)),
			roots:             roots,
		}
	})
}

type target struct {
	key              string
	path             string
	physicalBoundary string
	maxBytes         int64
	maxEntries       int
}

// The baseline pass records what exists without reporting it; every later pass
// reports changes except the Runtime's own accepted writes.
type reconcilePass int

const (
	baselinePass reconcilePass = iota
	observedPass
)

type baselineWatch interface {
	Observation
	reconcile(pass reconcilePass, accepted acceptance) error
}

func startObservation[T any, W baselineWatch](
	label string,
	canonical []T,
	boundary func(T) string,
	report func(error),
	assemble func(*observerLifecycle, *observationRoots) W,
) (Observation, error) {
	if len(canonical) == 0 {
		return nopWatch{}, nil
	}
	boundaries := make([]string, len(canonical))
	for index, candidate := range canonical {
		boundaries[index] = boundary(candidate)
	}
	roots, err := openObservationRoots(boundaries)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	lifecycle, err := newObserverLifecycle(label, report)
	if err != nil {
		return nil, errors.Join(err, roots.Close())
	}
	w := assemble(lifecycle, roots)
	if err := w.reconcile(baselinePass, acceptance{}); err != nil {
		return nil, errors.Join(lifecycle.abort(err), roots.Close())
	}
	lifecycle.start(func(accepted acceptance) error {
		return w.reconcile(observedPass, accepted)
	})
	return w, nil
}

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
	if candidate.MaxEntries < 0 {
		return target{}, fmt.Errorf("observe files: target %d entry limit must not be negative", index)
	}
	boundary, err := resolveTargetBoundary(index, candidate.Boundary)
	if err != nil {
		return target{}, err
	}
	return target{
		key: candidate.Key, path: filepath.Clean(candidate.Path),
		physicalBoundary: boundary, maxBytes: candidate.MaxBytes, maxEntries: candidate.MaxEntries,
	}, nil
}

func resolveTargetBoundary(index int, boundary string) (string, error) {
	if boundary == "" {
		return "", nil
	}
	if !filepath.IsAbs(boundary) {
		return "", fmt.Errorf("observe files: target %d boundary must be absolute", index)
	}
	resolved, err := filesystem.ResolvePath("", boundary)
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

func (w *watch) reconcile(pass reconcilePass, accepted acceptance) error {
	w.stateMu.Lock()
	if w.closedLocked() {
		w.stateMu.Unlock()
		return nil
	}
	paths := make(map[string]struct{})
	next := make([]fingerprint, len(w.targets))
	changedKeys := make([]string, 0, len(w.targets))
	accepting := len(accepted.keys) > 0 && len(accepted.identities) > 0
	for index, candidate := range w.targets {
		observed, err := fingerprintOf(candidate, w.roots)
		if err != nil {
			w.stateMu.Unlock()
			return err
		}
		match := noAcceptedWrite
		switch {
		case !accepting:
		case accepted.matches(candidate, observed.physical):
			match = acceptedWriteHere
		default:
			match = acceptedWriteElsewhere
		}
		var changed bool
		next[index], changed = advanceFingerprint(
			pass, match, w.fingerprints[index], observed.fingerprint,
		)
		if changed && !slices.Contains(changedKeys, candidate.key) {
			changedKeys = append(changedKeys, candidate.key)
		}
		for _, path := range observed.dependencies {
			paths[path] = struct{}{}
		}
		if err := collectParentDirectories(paths, candidate.path, observed.physical, candidate.physicalBoundary); err != nil {
			w.stateMu.Unlock()
			return err
		}
	}
	if err := w.replacePaths(paths); err != nil {
		w.stateMu.Unlock()
		return err
	}
	w.fingerprints = next
	w.stateMu.Unlock()
	if !accepting && len(changedKeys) > 0 {
		w.notify(changedKeys)
	}
	return nil
}

type acceptanceMatch uint8

const (
	noAcceptedWrite acceptanceMatch = iota + 1
	acceptedWriteElsewhere
	acceptedWriteHere
)

func advanceFingerprint(
	pass reconcilePass,
	match acceptanceMatch,
	previous fingerprint,
	observed fingerprint,
) (fingerprint, bool) {
	switch {
	case pass == baselinePass || match == acceptedWriteHere:
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

func fingerprintOf(candidate target, roots *observationRoots) (observedPath, error) {
	encoder := newFingerprintEncoder()
	encoder.field(fingerprintFieldLogicalPath, candidate.path)
	info, err := os.Lstat(candidate.path)
	if errors.Is(err, os.ErrNotExist) {
		encoder.state(fingerprintStateMissing)
		return observedPath{fingerprint: encoder.sum()}, nil
	}
	if err != nil {
		return observedPath{}, fmt.Errorf("observe files: inspect %q: %w", candidate.path, err)
	}
	encoder.fileInfo(fingerprintFieldLogicalInfo, info)
	if info.Mode()&os.ModeSymlink != 0 {
		destination, readErr := os.Readlink(candidate.path)
		if readErr != nil {
			return observedPath{}, fmt.Errorf("observe files: read symlink %q: %w", candidate.path, readErr)
		}
		encoder.field(fingerprintFieldLinkTarget, destination)
	}
	physical, err := filesystem.ResolvePath("", candidate.path)
	if err != nil {
		encoder.state(fingerprintStateUnresolved)
		encoder.field(fingerprintFieldError, err.Error())
		return observedPath{fingerprint: encoder.sum()}, nil
	}
	return fingerprintPhysicalTarget(encoder, candidate, physical, roots)
}

func fingerprintPhysicalTarget(
	encoder *fingerprintEncoder,
	candidate target,
	physical string,
	roots *observationRoots,
) (_ observedPath, err error) {
	observed, inside, err := roots.access(candidate.physicalBoundary, physical)
	if err != nil {
		return observedPath{}, fmt.Errorf("observe files: confine %q: %w", candidate.path, err)
	}
	if !inside {
		encoder.state(fingerprintStateOutsideBoundary)
		encoder.field(fingerprintFieldPhysicalPath, physical)
		return observedPath{fingerprint: encoder.sum()}, nil
	}
	encoder.field(fingerprintFieldPhysicalPath, physical)
	physicalInfo, err := observed.stat()
	if errors.Is(err, os.ErrNotExist) {
		encoder.state(fingerprintStateMissingTarget)
		return observedPath{fingerprint: encoder.sum(), physical: physical}, nil
	}
	if err != nil {
		return observedPath{}, fmt.Errorf("observe files: inspect target %q: %w", physical, err)
	}
	if physicalInfo.Mode().IsRegular() {
		if err := fingerprintRegularFile(encoder, observed, candidate, physical, physicalInfo); err != nil {
			return observedPath{}, err
		}
		return observedPath{fingerprint: encoder.sum(), physical: physical, dependencies: []string{physical}}, nil
	}
	encoder.fileInfo(fingerprintFieldPhysicalInfo, physicalInfo)
	var dependencies []string
	if physicalInfo.IsDir() && candidate.maxEntries > 0 {
		dependencies, err = fingerprintDirectory(encoder, observed, physical, candidate.maxEntries)
		if err != nil {
			return observedPath{}, err
		}
	}
	return observedPath{fingerprint: encoder.sum(), physical: physical, dependencies: dependencies}, nil
}

func fingerprintRegularFile(encoder *fingerprintEncoder, observed observedName, candidate target, physical string, info os.FileInfo) error {
	if info.Size() > candidate.maxBytes {
		encoder.fileInfo(fingerprintFieldPhysicalInfo, info)
		encoder.state(fingerprintStateTooLarge)
		return nil
	}
	file, opened, err := observed.openFile(info, candidate.maxBytes)
	if err != nil {
		return fmt.Errorf("observe files: open %q: %w", physical, err)
	}
	encoder.fileInfo(fingerprintFieldPhysicalInfo, opened)
	copyErr := encoder.content(file, opened.Size())
	versionErr := verifyObservedFileVersion(observed, file, opened)
	closeErr := file.Close()
	if copyErr != nil || versionErr != nil || closeErr != nil {
		return fmt.Errorf("observe files: read %q: %w", physical, errors.Join(copyErr, versionErr, closeErr))
	}
	return nil
}

func fingerprintDirectory(encoder *fingerprintEncoder, observed observedName, physical string, limit int) (dependencies []string, err error) {
	directory, err := observed.open()
	if err != nil {
		return nil, fmt.Errorf("observe files: open directory: %w", err)
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	entries, err := directory.ReadDir(limit + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("observe files: read directory: %w", err)
	}
	if len(entries) > limit {
		return nil, fmt.Errorf("observe files: directory exceeds %d immediate entries", limit)
	}
	dependencies = []string{physical}
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	for _, entry := range entries {
		info, err := observed.child(entry.Name()).lstat()
		if err != nil {
			return nil, fmt.Errorf("observe files: inspect directory entry: %w", err)
		}
		encoder.field(fingerprintFieldLogicalPath, entry.Name())
		encoder.fileInfo(fingerprintFieldChildInfo, info)
		dependencies = append(dependencies, metadataWatchPath(filepath.Join(physical, entry.Name()), info.Mode()))
	}
	return dependencies, nil
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
