package fileobservation

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestDirectoryObservationRejectsEntryOverflow(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one", "two"} {
		if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	observation, err := Watch([]Target{{Key: "root", Path: root, Boundary: root, MaxEntries: 1, MaxBytes: testMaxBytes}}, nil, discardOutage)
	if observation != nil || err == nil {
		t.Fatalf("overflow registration = (%v, %v)", observation, err)
	}
}

func TestDirectoryObservationDoesNotRecurseOrFollowChildSymlinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	nestedFile := filepath.Join(nested, "not-observed")
	if err := os.WriteFile(nestedFile, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	notified := make(chan []string, 8)
	observation, err := Watch([]Target{{Key: "root", Path: root, Boundary: root, MaxEntries: 10, MaxBytes: testMaxBytes}}, func(keys []string) { notified <- keys }, discardOutage)
	if err != nil {
		t.Fatal(err)
	}
	defer observation.Close()
	if err := os.WriteFile(filepath.Join(outside, "not-observed"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedFile, []byte("nested"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case keys := <-notified:
		t.Fatalf("recursed outside immediate entries: %v", keys)
	case <-time.After(3 * debounce):
	}
	if err := os.WriteFile(filepath.Join(root, "direct"), []byte("entry"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, notified, "root")
}

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
	watcher, err := Watch([]Target{{Key: "hooks", Path: target, MaxBytes: testMaxBytes}}, func(keys []string) { events <- keys }, discardOutage)
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
	target := filepath.Join(root, "document-target.md")
	alias := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(target, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Base(target), alias); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 8)
	watcher, err := Watch([]Target{{
		Key: "document", Path: alias, Boundary: root, MaxBytes: testMaxBytes,
	}}, func(keys []string) {
		events <- keys
	}, discardOutage)
	if err != nil {
		t.Fatalf("watch symlink: %v", err)
	}

	if err := os.WriteFile(target, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "document")
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
	targetPath := filepath.Join(directory, "AGENTS.md")
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
	if err := os.WriteFile(filepath.Join(outside, "AGENTS.md"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, directory); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	candidate := target{
		key: "document", path: targetPath, physicalBoundary: physicalBoundary, maxBytes: testMaxBytes,
	}
	if _, _, err := fingerprintPhysicalTarget(newFingerprintEncoder(), candidate, physical, roots); err == nil {
		t.Fatal("replaced target escaped its observation boundary")
	}
}

func TestAcceptRefreshesOnlyTheExactIdentity(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first", "AGENTS.md")
	second := filepath.Join(root, "second", "AGENTS.md")
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
		{Key: "document", Path: first, Boundary: filepath.Dir(first), MaxBytes: testMaxBytes},
		{Key: "document", Path: second, Boundary: filepath.Dir(second), MaxBytes: testMaxBytes},
	}, func(keys []string) { events <- keys }, discardOutage)
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
	if err := watcher.Accept([]string{"document"}, []string{first}); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "document")
	select {
	case keys := <-events:
		t.Fatalf("accepted identity produced a duplicate callback: %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestWatchSuppressesMetadataNoiseWithoutSemanticChange(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(target, []byte("stable"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 2)
	watcher, err := Watch([]Target{{
		Key: "document", Path: target, Boundary: root, MaxBytes: testMaxBytes,
	}}, func(keys []string) {
		events <- keys
	}, discardOutage)
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
	target := filepath.Join(root, "AGENTS.md")
	if err := os.WriteFile(target, []byte("oversized"), 0o644); err != nil {
		t.Fatal(err)
	}
	events := make(chan []string, 2)
	watcher, err := Watch([]Target{{
		Key: "document", Path: target, Boundary: root, MaxBytes: 1,
	}}, func(keys []string) { events <- keys }, discardOutage)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "document")
}

func TestCanonicalTargetsKeepBoundaryPolicyInIdentity(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "AGENTS.md")
	targets, err := canonicalTargets([]Target{
		{Key: "document", Path: path, Boundary: root, MaxBytes: testMaxBytes},
		{Key: "document", Path: path, Boundary: filepath.Dir(root), MaxBytes: testMaxBytes},
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
	path := filepath.Join(root, "AGENTS.md")
	_, err := canonicalTargets([]Target{
		{Key: "document", Path: path, Boundary: root, MaxBytes: testMaxBytes},
		{Key: "document", Path: path, Boundary: "relative", MaxBytes: testMaxBytes},
	})
	if err == nil {
		t.Fatal("canonical targets accepted an invalid boundary hidden behind a duplicate")
	}
}

func TestAdvanceFingerprintOwnsAcceptancePolicy(t *testing.T) {
	previous := fingerprint{1}
	observed := fingerprint{2}
	for _, test := range []struct {
		name        string
		initial     bool
		match       acceptanceMatch
		want        fingerprint
		wantChanged bool
	}{
		{name: "initial", initial: true, match: noAcceptedWrite, want: observed},
		{name: "initial during acceptance", initial: true, match: acceptedWriteElsewhere, want: observed},
		{name: "accepted identity", match: acceptedWriteHere, want: observed},
		{name: "unrelated during acceptance", match: acceptedWriteElsewhere, want: previous},
		{name: "ordinary external change", match: noAcceptedWrite, want: observed, wantChanged: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, changed := advanceFingerprint(
				test.initial, test.match, previous, observed,
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

// TestAcceptedWriteStaysAcceptedOnTheNextPass pins that accepting a write
// adopts that target's fingerprint rather than holding the one before it.
// Holding it suppresses the callback only until something else moves, and then
// reports the write the caller already knows about.
func TestAcceptedWriteStaysAcceptedOnTheNextPass(t *testing.T) {
	root := t.TempDir()
	accepted := filepath.Join(root, "accepted.md")
	other := filepath.Join(root, "other.md")
	for _, path := range []string{accepted, other} {
		if err := os.WriteFile(path, []byte("initial"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan []string, 4)
	watcher, err := Watch([]Target{
		{Key: "accepted", Path: accepted, Boundary: root, MaxBytes: testMaxBytes},
		{Key: "other", Path: other, Boundary: root, MaxBytes: testMaxBytes},
	}, func(keys []string) { events <- keys }, discardOutage)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = watcher.Close() }()

	if err := os.WriteFile(accepted, []byte("written through the api"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := watcher.Accept([]string{"accepted"}, []string{accepted}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("written outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertObservedKey(t, events, "other")
	select {
	case keys := <-events:
		t.Fatalf("the accepted write was reported on a later pass: %v", keys)
	case <-time.After(300 * time.Millisecond):
	}
}

// discardOutage is the inert reporter a test that does not exercise outage
// reporting supplies. An observer requires one, because the background loop is
// the only place a reconciliation outage is observable.
func discardOutage(error) {}

// TestObserverRequiresItsOutageReporter pins the collaborator an observer
// cannot watch without. A reconciliation outage is reported nowhere else, so an
// observation built without a reporter would retry in silence for as long as
// the failure lasts.
func TestObserverRequiresItsOutageReporter(t *testing.T) {
	root := t.TempDir()
	if _, err := Watch(
		[]Target{{Key: "document", Path: root, MaxBytes: testMaxBytes}},
		func([]string) {}, nil,
	); err == nil {
		t.Fatal("Watch accepted an observation with no outage reporter")
	}
	if _, err := WatchChildFiles(
		[]ChildFileTarget{{
			Key: "skills", Path: root, FileName: "SKILL.md",
			MaxEntries: 4, MaxBytes: testMaxBytes,
		}},
		func([]string) {}, nil,
	); err == nil {
		t.Fatal("WatchChildFiles accepted an observation with no outage reporter")
	}
}
