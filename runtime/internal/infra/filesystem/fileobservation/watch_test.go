package fileobservation

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

const testMaxBytes int64 = 1 << 20

func TestWatchCloseJoinsErrorReporting(t *testing.T) {
	parent := filepath.Join(t.TempDir(), ".flame")
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	watcher, err := Watch([]Target{{
		Key: "hooks", Path: filepath.Join(parent, "hooks.json"), MaxBytes: testMaxBytes,
	}}, nil, func(error) {
		started <- struct{}{}
		<-release
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		unblock()
		_ = watcher.Close()
	})
	if err := os.WriteFile(parent, []byte("parent is not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("error callback did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- watcher.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close returned before the callback completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not join the completed callback")
	}
}

func TestWatchObservesMissingParentsReplacementAndRemoval(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "nested", ".flame", "hooks.json")
	events := make(chan []string, 8)
	watcher, err := Watch([]Target{{Key: "hooks", Path: target, MaxBytes: testMaxBytes}}, func(keys []string) { events <- keys }, nil)
	if err != nil {
		t.Fatalf("watch missing target: %v", err)
	}
	defer func() { _ = watcher.Close() }()

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "hooks")

	replacement := filepath.Join(filepath.Dir(target), "replacement")
	if err := os.WriteFile(replacement, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, target); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "hooks")

	if err := os.RemoveAll(filepath.Join(root, "nested")); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "hooks")
}

func TestWatchObservesPhysicalSymlinkTargetAndCloseJoins(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "knowledge-target.md")
	alias := filepath.Join(root, "FLAME.md")
	if err := os.WriteFile(target, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), alias); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 8)
	watcher, err := Watch([]Target{{
		Key: "knowledge", Path: alias, Boundary: root, MaxBytes: testMaxBytes,
	}}, func(keys []string) {
		events <- keys
	}, nil)
	if err != nil {
		t.Fatalf("watch symlink: %v", err)
	}

	if err := os.WriteFile(target, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "knowledge")
	if err := watcher.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("three"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("callback after Close = %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestFingerprintPhysicalTargetRejectsEscapingReplacement(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "nested")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	targetPath := filepath.Join(directory, "FLAME.md")
	if err := os.WriteFile(targetPath, []byte("inside"), 0o644); err != nil {
		t.Fatal(err)
	}
	physical, err := filepath.EvalSymlinks(targetPath)
	if err != nil {
		t.Fatal(err)
	}
	physicalBoundary, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	roots, err := openObservationRoots([]string{physicalBoundary})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = roots.Close() }()
	if err := os.Remove(targetPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "FLAME.md"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, directory); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	candidate := target{
		key: "knowledge", path: targetPath, physicalBoundary: physicalBoundary, maxBytes: testMaxBytes,
	}
	if _, _, err := fingerprintPhysicalTarget(newFingerprintEncoder(), candidate, physical, roots); err == nil {
		t.Fatal("replaced target escaped its observation boundary")
	}
}

func TestAcceptRefreshesOnlyTheExactIdentity(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first", "FLAME.md")
	second := filepath.Join(root, "second", "FLAME.md")
	for _, path := range []string{first, second} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan []string, 4)
	watcher, err := Watch([]Target{
		{Key: "knowledge", Path: first, Boundary: filepath.Dir(first), MaxBytes: testMaxBytes},
		{Key: "knowledge", Path: second, Boundary: filepath.Dir(second), MaxBytes: testMaxBytes},
	}, func(keys []string) { events <- keys }, nil)
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
	if err := watcher.Accept([]string{"knowledge"}, []string{first}); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "knowledge")
	select {
	case keys := <-events:
		t.Fatalf("accepted identity produced a duplicate callback: %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchSuppressesMetadataNoiseWithoutSemanticChange(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "FLAME.md")
	if err := os.WriteFile(target, []byte("stable"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 2)
	watcher, err := Watch([]Target{{
		Key: "knowledge", Path: target, Boundary: root, MaxBytes: testMaxBytes,
	}}, func(keys []string) {
		events <- keys
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(target, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-events:
		t.Fatalf("metadata-only noise published %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchBoundsOversizedContentFingerprints(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "FLAME.md")
	if err := os.WriteFile(target, []byte("oversized"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 2)
	watcher, err := Watch([]Target{{
		Key: "knowledge", Path: target, Boundary: root, MaxBytes: 1,
	}}, func(keys []string) { events <- keys }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "knowledge")
}

func TestCanonicalTargetsKeepBoundaryPolicyInIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "FLAME.md")
	targets, err := canonicalTargets([]Target{
		{Key: "knowledge", Path: path, Boundary: root, MaxBytes: testMaxBytes},
		{Key: "knowledge", Path: path, Boundary: filepath.Dir(root), MaxBytes: testMaxBytes},
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

func TestCanonicalTargetsValidateEveryDuplicateCandidate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "FLAME.md")
	_, err := canonicalTargets([]Target{
		{Key: "knowledge", Path: path, Boundary: root, MaxBytes: testMaxBytes},
		{Key: "knowledge", Path: path, Boundary: "relative", MaxBytes: testMaxBytes},
	})
	if err == nil {
		t.Fatal("canonical targets accepted an invalid boundary hidden behind a duplicate")
	}
}

func TestAdvanceFingerprintOwnsAcceptancePolicy(t *testing.T) {
	previous := fingerprint{1}
	observed := fingerprint{2}
	for _, test := range []struct {
		name            string
		initial         bool
		accepting       bool
		matchesAccepted bool
		want            fingerprint
		wantChanged     bool
	}{
		{name: "initial", initial: true, want: observed},
		{name: "accepted identity", accepting: true, matchesAccepted: true, want: observed},
		{name: "unrelated during acceptance", accepting: true, want: previous},
		{name: "ordinary external change", want: observed, wantChanged: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, changed := advanceFingerprint(
				test.initial, test.accepting, test.matchesAccepted, previous, observed,
			)
			if got != test.want || changed != test.wantChanged {
				t.Fatalf("advance = (%x, %v), want (%x, %v)", got, changed, test.want, test.wantChanged)
			}
		})
	}
}

func assertObservedKey(t *testing.T, events <-chan []string, want string) {
	t.Helper()
	select {
	case keys := <-events:
		if len(keys) != 1 || keys[0] != want {
			t.Fatalf("keys = %v, want [%s]", keys, want)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("no %s observation", want)
	}
}
