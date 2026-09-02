package fileobservation

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/infra/filesystem/pathidentity"
)

func TestWatchChildFilesObservesDynamicExactFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "skills")
	events := make(chan []string, 8)
	watcher, err := WatchChildFiles([]ChildFileTarget{{
		Key: "skills", Path: root, Boundary: filepath.Dir(root), FileName: "SKILL.md",
		MaxEntries: 16, MaxBytes: testMaxBytes,
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()

	file := filepath.Join(root, "lint", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
	if err := os.WriteFile(file, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
	if err := os.RemoveAll(filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
}

func TestWatchChildFilesRequiresPositiveHardLimits(t *testing.T) {
	root := t.TempDir()
	for _, target := range []ChildFileTarget{
		{Key: "skills", Path: root, FileName: "SKILL.md", MaxBytes: testMaxBytes},
		{Key: "skills", Path: root, FileName: "SKILL.md", MaxEntries: 1},
	} {
		if _, err := WatchChildFiles([]ChildFileTarget{target}, nil); err == nil {
			t.Fatalf("WatchChildFiles accepted missing hard limit: %+v", target)
		}
	}
}

func TestCanonicalChildFileTargetsKeepBoundaryPolicyInIdentity(t *testing.T) {
	root := t.TempDir()
	targets, err := canonicalChildFileTargets([]ChildFileTarget{
		{Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md", MaxEntries: 16, MaxBytes: testMaxBytes},
		{Key: "skills", Path: root, Boundary: filepath.Dir(root), FileName: "SKILL.md", MaxEntries: 16, MaxBytes: testMaxBytes},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("canonical targets = %d, want both confinement policies", len(targets))
	}
	if targets[0].physicalBoundary == targets[1].physicalBoundary {
		t.Fatalf("canonical boundaries collapsed to %q", targets[0].physicalBoundary)
	}
}

func TestCanonicalChildFileTargetsValidateEveryDuplicateCandidate(t *testing.T) {
	root := t.TempDir()
	_, err := canonicalChildFileTargets([]ChildFileTarget{
		{Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md", MaxEntries: 16, MaxBytes: testMaxBytes},
		{Key: "skills", Path: root, Boundary: "relative", FileName: "SKILL.md", MaxEntries: 16, MaxBytes: testMaxBytes},
	})
	if err == nil {
		t.Fatal("canonical targets accepted an invalid boundary hidden behind a duplicate")
	}
}

func TestWatchChildFilesAcceptsOnlyExactCommittedFiles(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first", "SKILL.md")
	second := filepath.Join(root, "second", "SKILL.md")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan []string, 4)
	watcher, err := WatchChildFiles([]ChildFileTarget{{
		Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md",
		MaxEntries: 16, MaxBytes: testMaxBytes,
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	if err := os.WriteFile(first, []byte("api write"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("external write"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := watcher.Accept([]string{"skills"}, []string{first}); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
	select {
	case keys := <-events:
		t.Fatalf("accepted identity produced a duplicate callback: %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchChildFilesIgnoresNonProjectionFiles(t *testing.T) {
	root := t.TempDir()
	events := make(chan []string, 2)
	watcher, err := WatchChildFiles([]ChildFileTarget{{
		Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md",
		MaxEntries: 16, MaxBytes: testMaxBytes,
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	if err := os.WriteFile(filepath.Join(root, ".usage.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("non-projection file published %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchChildFilesDoesNotWedgeOnNonRegularProjectionPath(t *testing.T) {
	root := t.TempDir()
	events := make(chan []string, 4)
	watcher, err := WatchChildFiles([]ChildFileTarget{{
		Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md",
		MaxEntries: 16, MaxBytes: testMaxBytes,
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	path := filepath.Join(root, "broken", "SKILL.md")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("invalid projection directory published %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("now regular"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
}

func TestScanChildFilesIgnoresRootAndNestedMatches(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "lint")
	nested := filepath.Join(child, "references")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteObservedFile(t, filepath.Join(root, "SKILL.md"), "not a library entry")
	mustWriteObservedFile(t, filepath.Join(nested, "SKILL.md"), "not a library entry")
	target := childFileTarget{
		key: "skills", path: root,
		fileName: "SKILL.md", maxEntries: 16, maxBytes: testMaxBytes,
	}

	snapshot, watched := mustScanChildFiles(t, target)
	if len(snapshot.files) != 0 || snapshot.overflow {
		t.Fatalf("root/nested matches entered snapshot: %+v", snapshot)
	}
	physicalRoot, err := pathidentity.Resolve("", root)
	if err != nil {
		t.Fatal(err)
	}
	physicalChild := filepath.Join(physicalRoot, "lint")
	physicalNested := filepath.Join(physicalChild, "references")
	requireBoundedChildWatches(t, watched, physicalRoot, physicalChild, physicalNested)

	document := filepath.Join(child, "SKILL.md")
	mustWriteObservedFile(t, document, "library entry")
	snapshot, watched = mustScanChildFiles(t, target)
	if len(snapshot.files) != 1 || snapshot.overflow {
		t.Fatalf("immediate child snapshot = %+v, want one file", snapshot)
	}
	if _, present := snapshot.files[document]; !present {
		t.Fatalf("snapshot files = %v, want %q", snapshot.files, document)
	}
	requireBoundedChildWatches(t, watched, physicalRoot, physicalChild, physicalNested)
}

func mustWriteObservedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustScanChildFiles(t *testing.T, target childFileTarget) (childFileSnapshot, []string) {
	t.Helper()
	roots, err := openObservationRoots([]string{target.physicalBoundary})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = roots.Close() }()
	snapshot, watched, err := scanChildFiles(target, roots)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, watched
}

func TestScanChildFileDirectoryRejectsEscapingReplacement(t *testing.T) {
	boundary := t.TempDir()
	directory := filepath.Join(boundary, "skills")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	physicalBoundary, err := pathidentity.Resolve("", boundary)
	if err != nil {
		t.Fatal(err)
	}
	physicalDirectory, err := pathidentity.Resolve("", directory)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := openObservationRoots([]string{physicalBoundary})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = roots.Close() }()
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(outside, "external"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, directory); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	candidate := childFileTarget{
		key: "skills", path: directory, physicalBoundary: physicalBoundary,
		fileName: "SKILL.md", maxEntries: 16, maxBytes: testMaxBytes,
	}
	if _, _, err := scanChildFileDirectory(candidate, physicalDirectory, info, roots); err == nil {
		t.Fatal("replaced directory escaped its observation boundary")
	}
}

func TestFingerprintResolvedChildFileRejectsEscapingReplacement(t *testing.T) {
	boundary := t.TempDir()
	directory := filepath.Join(boundary, "skill")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	logical := filepath.Join(directory, "SKILL.md")
	if err := os.WriteFile(logical, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	resolved, err := filepath.EvalSymlinks(logical)
	if err != nil {
		t.Fatal(err)
	}
	physicalBoundary, err := pathidentity.Resolve("", boundary)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := openObservationRoots([]string{physicalBoundary})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = roots.Close() }()
	if err := os.Remove(logical); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, directory); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, _, err := fingerprintResolvedChildFile(
		logical, resolved, physicalBoundary, testMaxBytes, roots,
	); err == nil {
		t.Fatal("replaced child file escaped its observation boundary")
	}
}

func requireBoundedChildWatches(t *testing.T, watched []string, root, child, nested string) {
	t.Helper()
	if !slices.Contains(watched, root) || !slices.Contains(watched, child) || slices.Contains(watched, nested) {
		t.Fatalf("watched directories = %v, want root/immediate child without nested resource", watched)
	}
}

func TestWatchChildFilesObservesEntryLimitTransitions(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"first", "second", "third"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan []string, 4)
	watcher, err := WatchChildFiles([]ChildFileTarget{{
		Key: "skills", Path: root, Boundary: root, FileName: "SKILL.md",
		MaxEntries: 2, MaxBytes: testMaxBytes,
	}}, func(keys []string) { events <- keys })
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()

	if err := os.RemoveAll(filepath.Join(root, "third")); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
	if err := os.WriteFile(filepath.Join(root, "first", "SKILL.md"), []byte("now visible"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "skills")
}
