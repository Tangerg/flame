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
	boundaries := make([]string, len(canonical))
	for index, candidate := range canonical {
		boundaries[index] = candidate.physicalBoundary
	}
	roots, err := openObservationRoots(boundaries)
	if err != nil {
		return nil, fmt.Errorf("observe child files: %w", err)
	}
	lifecycle, err := newObserverLifecycle("observe child files")
	if err != nil {
		return nil, errors.Join(err, roots.Close())
	}
	w := &childFileWatch{
		observerLifecycle: lifecycle,
		targets:           canonical,
		notify:            notify,
		baselines:         make([]childFileSnapshot, len(canonical)),
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

type childFileTarget struct {
	key              string
	path             string
	physicalBoundary string
	fileName         string
	maxEntries       int
	maxBytes         int64
}

func canonicalChildFileTargets(targets []ChildFileTarget) ([]childFileTarget, error) {
	out := make([]childFileTarget, 0, len(targets))
	seen := make(map[childFileTarget]struct{}, len(targets))
	for index, candidate := range targets {
		canonical, err := canonicalChildFileTarget(index, candidate)
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

func canonicalChildFileTarget(index int, candidate ChildFileTarget) (childFileTarget, error) {
	if candidate.Key == "" {
		return childFileTarget{}, fmt.Errorf("observe child files: target %d key is required", index)
	}
	if candidate.Path == "" || !filepath.IsAbs(candidate.Path) {
		return childFileTarget{}, fmt.Errorf("observe child files: target %d path must be absolute", index)
	}
	if candidate.FileName == "" || filepath.Base(candidate.FileName) != candidate.FileName {
		return childFileTarget{}, fmt.Errorf("observe child files: target %d filename must be one path element", index)
	}
	if candidate.MaxEntries <= 0 {
		return childFileTarget{}, fmt.Errorf("observe child files: target %d entry limit must be positive", index)
	}
	if candidate.MaxBytes <= 0 {
		return childFileTarget{}, fmt.Errorf("observe child files: target %d byte limit must be positive", index)
	}
	boundary, err := resolveChildFileBoundary(index, candidate.Boundary)
	if err != nil {
		return childFileTarget{}, err
	}
	return childFileTarget{
		key: candidate.Key, path: filepath.Clean(candidate.Path), physicalBoundary: boundary,
		fileName: candidate.FileName, maxEntries: candidate.MaxEntries, maxBytes: candidate.MaxBytes,
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
	roots     *observationRoots
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
		state, watched, err := scanChildFiles(candidate, t.roots)
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

func scanChildFiles(candidate childFileTarget, roots *observationRoots) (childFileSnapshot, []string, error) {
	physical, info, directories, err := resolveChildFileRoot(candidate, roots)
	if err != nil {
		return childFileSnapshot{}, nil, err
	}
	if info == nil {
		return childFileSnapshot{}, directories, nil
	}
	snapshot, children, err := scanChildFileDirectory(candidate, physical, info, roots)
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

func resolveChildFileRoot(
	candidate childFileTarget,
	roots *observationRoots,
) (string, os.FileInfo, []string, error) {
	physical, err := pathidentity.Resolve("", candidate.path)
	if err != nil {
		return "", nil, nil, fmt.Errorf("observe child files: resolve %q: %w", candidate.path, err)
	}
	root, name, inside, err := roots.access(candidate.physicalBoundary, physical)
	if err != nil {
		return "", nil, nil, fmt.Errorf("observe child files: confine %q: %w", candidate.path, err)
	}
	if !inside {
		directory, directoryErr := nearestExistingDirectory(filepath.Dir(candidate.path))
		return "", nil, []string{directory}, directoryErr
	}
	var info os.FileInfo
	if root != nil {
		info, err = root.Stat(name)
	} else {
		info, err = os.Stat(name)
	}
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
	roots *observationRoots,
) (childFileSnapshot, []string, error) {
	entries, overflow, err := readChildFileEntries(
		roots, candidate.physicalBoundary, physical, info, candidate.maxEntries,
	)
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
			logical, observed, present, err := observeImmediateChildFile(
				candidate, directory, entry.Name(), roots,
			)
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
	roots *observationRoots,
) (string, childFileEntry, bool, error) {
	path := filepath.Join(directory, candidate.fileName)
	root, name, inside, err := roots.access(candidate.physicalBoundary, path)
	if err != nil {
		return "", childFileEntry{}, false, fmt.Errorf("observe child files: confine %q: %w", path, err)
	}
	if !inside {
		return "", childFileEntry{}, false, nil
	}
	var matched os.FileInfo
	if root != nil {
		matched, err = root.Lstat(name)
	} else {
		matched, err = os.Lstat(name)
	}
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
	value, resolved, err := fingerprintChildFile(
		logical, path, candidate.physicalBoundary, candidate.maxBytes, roots,
	)
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

func readChildFileEntries(
	roots *observationRoots,
	boundary, path string,
	info os.FileInfo,
	maxEntries int,
) ([]os.DirEntry, bool, error) {
	root, name, inside, err := roots.access(boundary, path)
	if err != nil {
		return nil, false, err
	}
	if !inside {
		return nil, false, errors.New("child file directory is outside its observation boundary")
	}
	var directory *os.File
	if root != nil {
		directory, _, err = fileinput.OpenDirectoryAtExpected(root, name, info)
	} else {
		directory, _, err = fileinput.OpenDirectoryExpected(name, info)
	}
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

func fingerprintChildFile(
	logical, physical, boundary string,
	maxBytes int64,
	roots *observationRoots,
) (fingerprint, string, error) {
	resolved, err := pathidentity.Resolve("", physical)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe child files: resolve file %q: %w", logical, err)
	}
	return fingerprintResolvedChildFile(logical, resolved, boundary, maxBytes, roots)
}

func fingerprintResolvedChildFile(
	logical, resolved, boundary string,
	maxBytes int64,
	roots *observationRoots,
) (fingerprint, string, error) {
	root, name, inside, err := roots.access(boundary, resolved)
	if err != nil {
		return fingerprint{}, "", fmt.Errorf("observe child files: confine file %q: %w", logical, err)
	}
	if !inside {
		return fingerprint{}, resolved, nil
	}
	var info os.FileInfo
	if root != nil {
		info, err = root.Stat(name)
	} else {
		info, err = os.Stat(name)
	}
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
	var file *os.File
	if root != nil {
		file, _, err = fileinput.OpenAtExpected(root, name, info, maxBytes)
	} else {
		file, _, err = fileinput.OpenExpected(name, info, maxBytes)
	}
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

func (t *childFileWatch) Close() error {
	return errors.Join(t.observerLifecycle.Close(), t.roots.Close())
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
