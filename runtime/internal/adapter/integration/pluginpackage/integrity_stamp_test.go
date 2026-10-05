//go:build linux || darwin

package pluginpackage

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// settle launches until the cached proof is settled. A filesystem whose clock
// ticks coarsely stamps entries changed just before a scan within its first
// tick, and only a scan in a later tick can vouch for them.
func settle(t *testing.T, r *Releases, digest fingerprint.Digest) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		root, err := r.currentRoot(t.Context(), digest)
		if err != nil {
			t.Fatal(err)
		}
		if err := root.Close(); err != nil {
			t.Fatal(err)
		}
		if integrity, found := r.cached(digest); found && integrity.settled() {
			return
		}
	}
	t.Fatal("the release did not settle")
}

// rewrite replaces name with same-size content and restores its mode and
// modification time, leaving the change time as the only trace.
func rewrite(t *testing.T, path, content string) {
	t.Helper()
	original, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, original.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, original.ModTime(), original.ModTime()); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchReusesIntegrityUntilTheTreeChanges(t *testing.T) {
	r := testReleases(t)
	release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest, "backend.sh": "printf original"}))
	if err != nil {
		t.Fatal(err)
	}
	launch := func() error {
		root, err := r.currentRoot(t.Context(), release.Digest())
		if err != nil {
			return err
		}
		return root.Close()
	}
	settle(t, r, release.Digest())
	verified, _ := verificationOf(t, r, release.Digest())
	for range 2 {
		if err := launch(); err != nil {
			t.Fatal(err)
		}
	}
	if again, _ := verificationOf(t, r, release.Digest()); again != verified {
		t.Fatal("an unchanged release was scanned again for a launch")
	}

	root, err := r.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(root, "backend.sh")
	rewrite(t, backend, "printf tampers")
	if err := launch(); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("launch of same-size, same-mtime tampered bytes = %v", err)
	}
	if _, found := verificationOf(t, r, release.Digest()); found {
		t.Fatal("tampered bytes kept their cached integrity")
	}

	rewrite(t, backend, "printf original")
	if err := launch(); err != nil {
		t.Fatalf("repaired release did not launch: %v", err)
	}
	settle(t, r, release.Digest())
	repaired, _ := verificationOf(t, r, release.Digest())
	if err := launch(); err != nil {
		t.Fatal(err)
	}
	if again, _ := verificationOf(t, r, release.Digest()); again != repaired {
		t.Fatal("the repaired release was scanned again although it did not change")
	}
}

func TestLaunchRejectsARewriteStampedInTheScansTick(t *testing.T) {
	r := testReleases(t)
	release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest, "backend.sh": "printf original"}))
	if err != nil {
		t.Fatal(err)
	}
	root, err := r.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(root, "backend.sh")
	rewrite(t, backend, "printf tampers")
	info, err := os.Lstat(backend)
	if err != nil {
		t.Fatal(err)
	}
	stamp, stamped := stampOf(info)
	if !stamped {
		t.Fatal("no change stamp on a platform that trusts it")
	}
	// A filesystem with a coarse clock stamps a rewrite within the tick of the
	// scan that read the original bytes with the stamp that scan recorded.
	r.mu.Lock()
	proof := r.verified[release.Digest()]
	proof.stamps = maps.Clone(proof.stamps)
	proof.stamps["backend.sh"] = stamp
	r.verified[release.Digest()] = proof
	r.mu.Unlock()
	launched, err := r.currentRoot(t.Context(), release.Digest())
	if err == nil {
		_ = launched.Close()
	}
	if !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("launch of a rewrite that kept the recorded stamp = %v", err)
	}
}

func TestLaunchRejectsAScanThatReadTheTreeBeforeItChanged(t *testing.T) {
	r := testReleases(t)
	release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest, "backend.sh": "printf original"}))
	if err != nil {
		t.Fatal(err)
	}
	digest := release.Digest()
	stale, _ := r.cached(digest)
	root, err := r.Root(digest)
	if err != nil {
		t.Fatal(err)
	}
	synctest.Test(t, func(t *testing.T) {
		// The scan in flight read the original bytes before the rewrite.
		earlier := &verification{done: make(chan struct{}), integrity: stale}
		r.mu.Lock()
		r.verifying[digest] = earlier
		r.mu.Unlock()
		rewrite(t, filepath.Join(root, "backend.sh"), "printf tampers")
		launched := make(chan error, 1)
		go func() {
			root, err := r.currentRoot(t.Context(), digest)
			if err == nil {
				err = errors.Join(errors.New("launched"), root.Close())
			}
			launched <- err
		}()
		synctest.Wait()
		r.mu.Lock()
		r.verified[digest] = stale
		delete(r.verifying, digest)
		close(earlier.done)
		r.mu.Unlock()
		if err := <-launched; !errors.Is(err, plugin.ErrUnavailable) {
			t.Fatalf("launch that joined a scan begun before the rewrite = %v", err)
		}
	})
	if _, found := r.cached(digest); found {
		t.Fatal("a proof read before the rewrite stayed cached")
	}
}

func TestLaunchDetectsAnAddedEntry(t *testing.T) {
	r := testReleases(t)
	release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest}))
	if err != nil {
		t.Fatal(err)
	}
	root, err := r.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "extra.sh"), []byte("exit 0"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.currentRoot(t.Context(), release.Digest()); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("launch with an added entry = %v", err)
	}
}

func TestPublicationRecordsStampsALaunchTrusts(t *testing.T) {
	r := testReleases(t)
	release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest, "bin/server": "#!/bin/sh\n"}))
	if err != nil {
		t.Fatal(err)
	}
	publication, found := r.cached(release.Digest())
	if !found {
		t.Fatal("publication did not record the integrity it established")
	}
	root, err := r.currentRoot(t.Context(), release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	// Sealing changes every entry just before the digest; only where the
	// filesystem clock ticked in between can the launch trust those stamps.
	launched, _ := verificationOf(t, r, release.Digest())
	if publication.settled() && launched != publication.verifiedAt {
		t.Fatal("the first launch rescanned a release sealed before it was digested")
	}
	if !publication.settled() && launched == publication.verifiedAt {
		t.Fatal("the first launch trusted stamps sealed within the tick of the digest")
	}
}
