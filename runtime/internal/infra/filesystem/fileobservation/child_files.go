package fileobservation

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/fileinput"
	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

// ChildFileTarget identifies one bounded directory and the exact file directly
// below each immediate child directory whose contents define its observable
// projection. Other files and directory metadata are deliberately ignored.
// MaxEntries and MaxBytes are required hard bounds.
type ChildFileTarget struct {
	Key        string
	Path       string
	Boundary   string
	FileName   string
	MaxEntries int
	MaxBytes   int64
}

// WatchChildFiles observes a dynamic set of exact child files. It watches the
// root, each admitted immediate child directory, and the nearest existing
// ancestor of a missing root, so additions, replacements, removals, and
// in-place writes converge through one content-derived baseline.
func WatchChildFiles(targets []ChildFileTarget, notify func([]string)) (Observation, error) {
	canonical, err := canonicalChildFileTargets(targets)
	if err != nil {
		return nil, err
	}
	if len(canonical) == 0 {
		return nopWatch{}, nil
	}
	lifecycle, err := newObserverLifecycle("observe child files")
	if err != nil {
		return nil, err
	}
	w := &childFileWatch{
		observerLifecycle: lifecycle,
		targets:           canonical,
		notify:            notify,
		baselines:         make([]childFileSnapshot, len(canonical)),
	}
	if err := w.reconcile(true, acceptance{}); err != nil {
		return nil, lifecycle.abort(err)
	}
	lifecycle.start(func(accepted acceptance) error {
		return w.reconcile(false, accepted)
	})
	return w, nil
}

type childFileTarget struct {
	key              string
	path             string
	physicalBoundary string
	fileName         string
	maxEntries       int
	maxBytes         int64
}

type childFileTargetKey struct {
	name       string
	path       string
	fileName   string
	maxEntries int
	maxBytes   int64
}

func canonicalChildFileTargets(targets []ChildFileTarget) ([]childFileTarget, error) {
	out := make([]childFileTarget, 0, len(targets))
	seen := make(map[childFileTargetKey]struct{}, len(targets))
	for index, candidate := range targets {
		identity, err := validateChildFileTarget(index, candidate)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		boundary, err := resolveChildFileBoundary(index, candidate.Boundary)
		if err != nil {
			return nil, err
		}
		out = append(out, childFileTarget{
			key: candidate.Key, path: identity.path, physicalBoundary: boundary,
			fileName: candidate.FileName, maxEntries: candidate.MaxEntries, maxBytes: candidate.MaxBytes,
		})
	}
	return out, nil
}

func validateChildFileTarget(index int, candidate ChildFileTarget) (childFileTargetKey, error) {
	if candidate.Key == "" {
		return childFileTargetKey{}, fmt.Errorf("observe child files: target %d key is required", index)
	}
	if candidate.Path == "" || !filepath.IsAbs(candidate.Path) {
		return childFileTargetKey{}, fmt.Errorf("observe child files: target %d path must be absolute", index)
	}
	if candidate.FileName == "" || filepath.Base(candidate.FileName) != candidate.FileName {
		return childFileTargetKey{}, fmt.Errorf("observe child files: target %d filename must be one path element", index)
	}
	if candidate.MaxEntries <= 0 {
		return childFileTargetKey{}, fmt.Errorf("observe child files: target %d entry limit must be positive", index)
	}
	if candidate.MaxBytes <= 0 {
		return childFileTargetKey{}, fmt.Errorf("observe child files: target %d byte limit must be positive", index)
	}
	return childFileTargetKey{
		name: candidate.Key, path: filepath.Clean(candidate.Path), fileName: candidate.FileName,
		maxEntries: candidate.MaxEntries, maxBytes: candidate.MaxBytes,
	}, nil
}

func resolveChildFileBoundary(index int, boundary string) (string, error) {
	if boundary == "" {
		return "", nil
	}
	if !filepath.IsAbs(boundary) {
		return "", fmt.Errorf("observe child files: target %d boundary must be absolute", index)
	}
	resolved, err := pathidentity.Resolve("", boundary)
	if err != nil {
		return "", fmt.Errorf("observe child files: resolve target %d boundary: %w", index, err)
	}
	return resolved, nil
}

type childFileEntry struct {
	fingerprint fingerprint
	physical    string
}

type childFileSnapshot struct {
	files    map[string]childFileEntry
	overflow bool
}

type childFileWatch struct {
	*observerLifecycle
	targets   []childFileTarget
	notify    func([]string)
	baselines []childFileSnapshot
}

func (t *childFileWatch) reconcile(initial bool, accepted acceptance) error {
	t.stateMu.Lock()
	if t.closed {
		t.stateMu.Unlock()
		return nil
	}
	directories := make(map[string]struct{})
	next := make([]childFileSnapshot, len(t.targets))
	changedKeys := make([]string, 0, len(t.targets))
	accepting := len(accepted.keys) > 0 && len(accepted.identities) > 0
	for index, candidate := range t.targets {
		state, watched, err := scanChildFiles(candidate)
		if err != nil {
			t.stateMu.Unlock()
			return err
		}
		for _, directory := range watched {
			directories[directory] = struct{}{}
		}
		switch {
		case initial:
			next[index] = state
		case accepting:
			next[index] = acceptChildFileChanges(candidate, t.baselines[index], state, accepted)
		default:
			next[index] = state
			if !childFileSnapshotsEqual(state, t.baselines[index]) && !slices.Contains(changedKeys, candidate.key) {
				changedKeys = append(changedKeys, candidate.key)
			}
		}
	}
	if err := t.replaceDirectories(directories); err != nil {
		t.stateMu.Unlock()
		return err
	}
	t.baselines = next
	t.stateMu.Unlock()
	if !accepting && len(changedKeys) > 0 && t.notify != nil {
		t.notify(changedKeys)
	}
	return nil
}

func scanChildFiles(candidate childFileTarget) (childFileSnapshot, []string, error) {
	physical, info, directories, err := resolveChildFileRoot(candidate)
	if err != nil {
		return childFileSnapshot{}, nil, err
	}
	if info == nil {
		return childFileSnapshot{}, directories, nil
	}
	snapshot, children, err := scanChildFileDirectory(candidate, physical, info)
	if err != nil {
		return childFileSnapshot{}, nil, err
	}
	directories = append(directories, children...)
	logicalParent, err := childFileLogicalParent(candidate.path, physical)
	if err != nil {
		return childFileSnapshot{}, nil, err
	}
	if logicalParent != "" {
		directories = append(directories, logicalParent)
	}
	return snapshot, directories, nil
}

func resolveChildFileRoot(candidate childFileTarget) (string, os.FileInfo, []string, error) {
	physical, err := pathidentity.Resolve("", candidate.path)
	if err != nil {
		return "", nil, nil, fmt.Errorf("observe child files: resolve %q: %w", candidate.path, err)
	}
	if candidate.physicalBoundary != "" {
		inside, containsErr := pathidentity.Contains(candidate.physicalBoundary, physical)
		if containsErr != nil {
			return "", nil, nil, fmt.Errorf("observe child files: confine %q: %w", candidate.path, containsErr)
		}
		if !inside {
			directory, err := nearestExistingDirectory(filepath.Dir(candidate.path))
			return "", nil, []string{directory}, err
		}
	}
	info, err := os.Stat(physical)
	if errors.Is(err, os.ErrNotExist) {
		directory, directoryErr := nearestExistingDirectory(filepath.Dir(physical))
		return "", nil, []string{directory}, directoryErr
	}
	if err != nil {
		return "", nil, nil, fmt.Errorf("observe child files: inspect %q: %w", physical, err)
	}
	if !info.IsDir() {
		directory, directoryErr := nearestExistingDirectory(filepath.Dir(physical))
		return "", nil, []string{directory}, directoryErr
	}
	return physical, info, nil, nil
}

func scanChildFileDirectory(
	candidate childFileTarget,
	physical string,
	info os.FileInfo,
) (childFileSnapshot, []string, error) {
	entries, overflow, err := readChildFileEntries(physical, info, candidate.maxEntries)
	if err != nil {
		return childFileSnapshot{}, nil, fmt.Errorf("observe child files: scan %q: %w", physical, err)
	}
	snapshot := childFileSnapshot{files: make(map[string]childFileEntry), overflow: overflow}
	directories := []string{physical}
	if !overflow {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			directory := filepath.Join(physical, entry.Name())
			directories = append(directories, directory)
			logical, observed, present, err := observeImmediateChildFile(candidate, directory, entry.Name())
			if err != nil {
				return childFileSnapshot{}, nil, err
			}
			if present {
				snapshot.files[logical] = observed
			}
		}
	}
	return snapshot, directories, nil
}

func observeImmediateChildFile(
	candidate childFileTarget,
	directory string,
	childName string,
) (string, childFileEntry, bool, error) {
	path := filepath.Join(directory, candidate.fileName)
	matched, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", childFileEntry{}, false, nil
	}
	if err != nil {
		return "", childFileEntry{}, false, fmt.Errorf("observe child files: inspect %q: %w", path, err)
	}
	if matched.IsDir() {
		return "", childFileEntry{}, false, nil
	}
	logical := filepath.Join(candidate.path, childName, candidate.fileName)
	value, resolved, err := fingerprintChildFile(logical, path, candidate.physicalBoundary, candidate.maxBytes)
	if err != nil {
		return "", childFileEntry{}, false, err
	}
	return logical, childFileEntry{fingerprint: value, physical: resolved}, true, nil
}

func childFileLogicalParent(logical, physical string) (string, error) {
	if logical == physical {
		return "", nil
	}
	return nearestExistingDirectory(filepath.Dir(logical))
}

func readChildFileEntries(path string, info os.FileInfo, maxEntries int) ([]os.DirEntry, bool, error) {
	directory, _, err := fileinput.OpenDirectoryExpected(path, info)
	if err != nil {
		return nil, false, err
	}
	entries, readErr := directory.ReadDir(maxEntries + 1)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	closeErr := directory.Close()
	if readErr != nil || closeErr != nil {
		return nil, false, errors.Join(readErr, closeErr)
	}
	if len(entries) > maxEntries {
		return nil, true, nil
	}
	slices.SortFunc(entries, func(left, right os.DirEntry) int {
		return strings.Compare(left.Name(), right.Name())
	})
	return entries, false, nil
}

func fingerprintChildFile(logical, physical, boundary string, maxBytes int64) (fingerprint, string, error) {
	resolved, err := pathidentity.Resolve("", physical)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe child files: resolve file %q: %w", logical, err)
	}
	if boundary != "" {
		inside, containsErr := pathidentity.Contains(boundary, resolved)
		if containsErr != nil {
			return fingerprint{}, "", fmt.Errorf("observe child files: confine file %q: %w", logical, containsErr)
		}
		if !inside {
			return fingerprint{}, resolved, nil
		}
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe child files: inspect file %q: %w", logical, err)
	}
	if !info.Mode().IsRegular() {
		encoder := newFingerprintEncoder()
		encoder.field(fingerprintFieldLogicalPath, logical)
		encoder.field(fingerprintFieldPhysicalPath, resolved)
		encoder.fileInfo(fingerprintFieldChildInfo, info)
		return encoder.sum(), resolved, nil
	}
	if info.Size() > maxBytes {
		encoder := newFingerprintEncoder()
		encoder.field(fingerprintFieldLogicalPath, logical)
		encoder.field(fingerprintFieldPhysicalPath, resolved)
		encoder.fileInfo(fingerprintFieldChildInfo, info)
		encoder.state(fingerprintStateTooLarge)
		return encoder.sum(), resolved, nil
	}
	file, _, err := fileinput.OpenExpected(resolved, info, maxBytes)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe child files: open %q: %w", logical, err)
	}
	encoder := newFingerprintEncoder()
	encoder.field(fingerprintFieldLogicalPath, logical)
	encoder.field(fingerprintFieldPhysicalPath, resolved)
	copyErr := encoder.content(file)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return fingerprint{}, "", fmt.Errorf("observe child files: read %q: %w", logical, errors.Join(copyErr, closeErr))
	}
	return encoder.sum(), resolved, nil
}

func acceptChildFileChanges(
	candidate childFileTarget,
	baseline, current childFileSnapshot,
	accepted acceptance,
) childFileSnapshot {
	next := childFileSnapshot{files: make(map[string]childFileEntry, len(baseline.files)), overflow: baseline.overflow}
	for logical, entry := range baseline.files {
		next.files[logical] = entry
	}
	if !accepted.keys[candidate.key] {
		return next
	}
	// Capacity overflow is one opaque invalid source state. Retaining its prior
	// baseline may produce one conservative duplicate notification when an
	// accepted write enters or leaves overflow, but cannot hide an unrelated
	// external change whose individual identity was never observed.
	if baseline.overflow || current.overflow {
		return next
	}
	for logical, entry := range baseline.files {
		if accepted.identities[logical] || accepted.identities[entry.physical] {
			delete(next.files, logical)
		}
	}
	for logical, entry := range current.files {
		if accepted.identities[logical] || accepted.identities[entry.physical] {
			next.files[logical] = entry
		}
	}
	return next
}

func childFileSnapshotsEqual(left, right childFileSnapshot) bool {
	if left.overflow != right.overflow || len(left.files) != len(right.files) {
		return false
	}
	for logical, leftEntry := range left.files {
		rightEntry, ok := right.files[logical]
		if !ok || leftEntry != rightEntry {
			return false
		}
	}
	return true
}
