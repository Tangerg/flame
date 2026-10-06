package pluginpackage

import (
	"context"
	"crypto/sha256"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

func verificationOf(t *testing.T, r *Releases, digest fingerprint.Digest) (uint64, bool) {
	t.Helper()
	integrity, found := r.cached(digest)
	return integrity.verifiedAt, found
}

func drop(r *Releases, digest fingerprint.Digest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.verified, digest)
}

func TestColdVerificationIsSharedPerDigestAndIndependentAcrossDigests(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := testReleases(t)
		first, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest, "notes.txt": "first"}))
		if err != nil {
			t.Fatal(err)
		}
		second, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest, "notes.txt": "second"}))
		if err != nil {
			t.Fatal(err)
		}
		drop(r, first.Digest())
		drop(r, second.Digest())
		inFlight := func(err error) func() {
			running := &verification{done: make(chan struct{}), err: err}
			r.mu.Lock()
			r.verifying[first.Digest()] = running
			r.mu.Unlock()
			return func() {
				r.mu.Lock()
				delete(r.verifying, first.Digest())
				r.mu.Unlock()
				close(running.done)
			}
		}

		shared := errors.New("shared verification outcome")
		finish := inFlight(shared)
		joined := make(chan error, 1)
		go func() {
			_, err := r.integrity(t.Context(), first.Digest())
			joined <- err
		}()
		if _, err := r.integrity(t.Context(), second.Digest()); err != nil {
			t.Fatalf("another release waited behind a cold verification: %v", err)
		}
		synctest.Wait()
		select {
		case err := <-joined:
			t.Fatalf("a caller finished before the verification it joined: %v", err)
		default:
		}
		finish()
		if err := <-joined; !errors.Is(err, shared) || !errors.Is(err, plugin.ErrUnavailable) {
			t.Fatalf("joined caller = %v, want the shared outcome", err)
		}
		if _, found := verificationOf(t, r, first.Digest()); found {
			t.Fatal("a joined caller scanned the release again")
		}

		finish = inFlight(context.Canceled)
		go func() {
			_, err := r.integrity(t.Context(), first.Digest())
			joined <- err
		}()
		synctest.Wait()
		finish()
		if err := <-joined; err != nil {
			t.Fatalf("a scan canceled by its own caller decided another caller's outcome: %v", err)
		}
		if _, found := verificationOf(t, r, first.Digest()); !found {
			t.Fatal("the surviving caller did not verify the release itself")
		}

		ctx, cancel := context.WithCancel(t.Context())
		finish = inFlight(nil)
		defer finish()
		cancel()
		if _, err := r.integrity(ctx, first.Digest()); !errors.Is(err, context.Canceled) {
			t.Fatalf("a canceled caller kept waiting for another scan: %v", err)
		}
	})
}

func TestLeadershipRechecksTheCacheAndHandsOffToOneScan(t *testing.T) {
	r := testReleases(t)
	release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest}))
	if err != nil {
		t.Fatal(err)
	}
	digest := release.Digest()
	published, _ := verificationOf(t, r, digest)
	if _, err := r.verifyAfter(t.Context(), digest, 0); err != nil {
		t.Fatal(err)
	}
	if again, _ := verificationOf(t, r, digest); again != published {
		t.Fatal("a caller that took leadership scanned although a sufficient proof was cached")
	}

	drop(r, digest)
	synctest.Test(t, func(t *testing.T) {
		abandoned := &verification{done: make(chan struct{}), err: context.Canceled}
		r.mu.Lock()
		r.verifying[digest] = abandoned
		r.mu.Unlock()
		const callers = 8
		proofs := make(chan releaseIntegrity, callers)
		for range callers {
			go func() {
				integrity, err := r.integrity(t.Context(), digest)
				if err != nil {
					t.Error(err)
				}
				proofs <- integrity
			}()
		}
		synctest.Wait()
		r.mu.Lock()
		delete(r.verifying, digest)
		close(abandoned.done)
		r.mu.Unlock()
		first := <-proofs
		for range callers - 1 {
			if proof := <-proofs; proof.verifiedAt != first.verifiedAt {
				t.Fatalf("callers handed leadership ran more than one scan: proofs %d and %d", first.verifiedAt, proof.verifiedAt)
			}
		}
	})
	if cached, _ := verificationOf(t, r, digest); cached == 0 {
		t.Fatal("the handed-off scan was not cached")
	}
}

func TestIntegrityCacheStaysWithinItsBoundAndKeepsHeldProofs(t *testing.T) {
	r := testReleases(t)
	proof := releaseIntegrity{treeContents: treeContents{files: map[string][sha256.Size]byte{strings.Repeat("n", maxVerifiedBytes/4): {}}}}
	fits := int(maxVerifiedBytes / proof.estimate())
	digests := make([]fingerprint.Digest, fits+2)
	for index := range digests {
		digests[index] = fingerprint.Sum([sha256.Size]byte{byte(index + 1)})
	}
	hold := func(digest fingerprint.Digest) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.verifying[digest] = &verification{done: make(chan struct{})}
	}
	within := func() {
		t.Helper()
		r.mu.Lock()
		defer r.mu.Unlock()
		var used int64
		for _, cached := range r.verified {
			used += cached.footprint
		}
		if used > maxVerifiedBytes {
			t.Fatalf("integrity cache retains %d bytes, bound is %d", used, maxVerifiedBytes)
		}
	}
	cached := func(digest fingerprint.Digest) bool {
		_, found := r.cached(digest)
		return found
	}

	for _, digest := range digests[:fits] {
		r.remember(digest, proof)
	}
	for _, digest := range digests[:fits] {
		if !cached(digest) {
			t.Fatal("a proof within the bound was not cached")
		}
	}
	hold(digests[0])
	r.remember(digests[fits], proof)
	within()
	if !cached(digests[0]) {
		t.Fatal("eviction dropped the oldest proof although a verification of it is in flight")
	}
	if cached(digests[1]) || !cached(digests[fits]) {
		t.Fatal("eviction did not replace the oldest proof nobody holds")
	}

	for _, digest := range digests[:fits+1] {
		if cached(digest) {
			hold(digest)
		}
	}
	r.remember(digests[fits+1], proof)
	within()
	if cached(digests[fits+1]) {
		t.Fatal("a proof was cached although only held proofs could make room")
	}
	for _, digest := range append([]fingerprint.Digest{digests[0]}, digests[2:fits+1]...) {
		if !cached(digest) {
			t.Fatal("eviction dropped a held proof")
		}
	}
}

func TestPublishedReleaseIsSealedReadOnly(t *testing.T) {
	r := testReleases(t)
	release, err := publishPackage(t.Context(), r, writePackage(t, map[string]string{"plugin.json": portableManifest, "bin/server": "#!/bin/sh\n"}))
	if err != nil {
		t.Fatal(err)
	}
	root, err := r.Root(release.Digest())
	if err != nil {
		t.Fatal(err)
	}
	if err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().Perm()&0222 != 0 {
			t.Errorf("%s is writable after publication: %v", name, info.Mode())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLaunchFromAMissingReleaseIsUnavailable(t *testing.T) {
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
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if _, err := r.currentRoot(t.Context(), release.Digest()); !errors.Is(err, plugin.ErrUnavailable) {
		t.Fatalf("launch from a removed release = %v", err)
	}
	if _, found := r.cached(release.Digest()); found {
		t.Fatal("a failed verification kept vouching for the release")
	}
}
