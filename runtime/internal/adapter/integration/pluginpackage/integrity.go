package pluginpackage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

// maxVerifiedBytes bounds the heap the integrity cache retains; it is the
// cache's own limit, not a count of releases. A proof holds each entry name
// once, rounded up to an allocation size class (at most an eighth for long
// names), and one slot in each of two maps that keep at least 7/16 of their
// slots full: at most 261 B per entry plus the rounding of short names.
// BenchmarkProofFootprint measures 260-290 B per entry for dependency-shaped
// trees and fails if the estimate undercounts. 32 MiB holds a proof for each of
// the MaxInstallations selected releases at about 750 entries, or 24 releases
// at the entry limit. A proof that does not fit is not cached, so its next
// launch rescans.
const (
	maxVerifiedBytes = 32 << 20
	proofEntryBytes  = 288
	proofBytes       = 1 << 10
)

// treeContents is what one full scan learned about a release tree: the content
// hash of every file and the change stamp of every entry when it was read.
// stamps is nil on a platform without a change stamp the Runtime user cannot
// set, where nothing short of a full scan can show the tree is unchanged.
// scanned is the filesystem's change time when the scan began.
type treeContents struct {
	files   map[string][sha256.Size]byte
	stamps  map[string]entryStamp
	scanned int64
}

// releaseIntegrity is the cached proof that a release directory held exactly
// its digest's bytes. It retains only byte integrity, never another
// interpretation of an admitted declaration.
type releaseIntegrity struct {
	treeContents
	directory fs.FileInfo
	// started is the generation at which the proving scan began. Publication
	// digests staging before any generation is assigned, so its proof keeps
	// zero, which satisfies no caller that observed a change.
	started    uint64
	verifiedAt uint64
	footprint  int64
}

// verification is one full scan of a digest in flight. Callers for the same
// digest share it; scans of different digests proceed independently.
type verification struct {
	done      chan struct{}
	started   uint64
	integrity releaseIntegrity
	err       error
}

// estimate is the heap a cached proof retains. The two maps share each name.
func (c treeContents) estimate() int64 {
	size := int64(proofBytes)
	entry := func(name string) int64 { return int64(len(name)+len(name)/8) + proofEntryBytes }
	for name := range c.files {
		size += entry(name)
	}
	for name := range c.stamps {
		if _, file := c.files[name]; !file {
			size += entry(name)
		}
	}
	return size
}

func (r *Releases) remember(digest fingerprint.Digest, integrity releaseIntegrity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commit(digest, integrity)
}

// commit records integrity as the newest proof for digest within
// maxVerifiedBytes, evicting the oldest proofs. A proof whose digest has a
// verification in flight is never evicted: that verification decides its fate,
// and readers that accept any verified proof are served by it meanwhile. When
// only such proofs could make room, the new proof is returned uncached. r.mu is
// held.
func (r *Releases) commit(digest fingerprint.Digest, integrity releaseIntegrity) releaseIntegrity {
	r.nextVerification++
	integrity.verifiedAt = r.nextVerification
	integrity.footprint = integrity.estimate()
	delete(r.verified, digest)
	var used, evictable int64
	for key, value := range r.verified {
		used += value.footprint
		if _, held := r.verifying[key]; !held {
			evictable += value.footprint
		}
	}
	if used-evictable+integrity.footprint > maxVerifiedBytes {
		return integrity
	}
	for used+integrity.footprint > maxVerifiedBytes {
		var oldest fingerprint.Digest
		for key, value := range r.verified {
			if _, held := r.verifying[key]; !held && (oldest.IsZero() || value.verifiedAt < r.verified[oldest].verifiedAt) {
				oldest = key
			}
		}
		used -= r.verified[oldest].footprint
		delete(r.verified, oldest)
	}
	r.verified[digest] = integrity
	return integrity
}

// withdraw drops the cached proof for digest when it is still the one a
// caller found stale, so a newer proof is never discarded on its behalf.
func (r *Releases) withdraw(digest fingerprint.Digest, stale releaseIntegrity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, found := r.verified[digest]; found && cached.verifiedAt == stale.verifiedAt {
		delete(r.verified, digest)
	}
}

// forget drops the cached proof of a release being reclaimed.
func (r *Releases) forget(digest fingerprint.Digest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.verified, digest)
}

func (r *Releases) cached(digest fingerprint.Digest) (releaseIntegrity, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	integrity, found := r.verified[digest]
	return integrity, found
}

// observe returns the oldest generation a verification may have started at to
// postdate everything the caller has observed so far.
func (r *Releases) observe() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.nextVerification + 1
}

// verify returns a full scan of digest that began no earlier than the call,
// so it vouches for the bytes as they are now.
func (r *Releases) verify(ctx context.Context, digest fingerprint.Digest) (releaseIntegrity, error) {
	return r.verifyAfter(ctx, digest, r.observe())
}

// verifyAfter returns a proof for digest whose scan started at generation
// since or later, reusing a cached proof, joining a scan in flight or leading
// a new one. A scan that began earlier may have read bytes the caller has
// since seen change, so a caller arriving during it waits and then scans
// again. A cold 128 MiB release delays only the callers that need it.
func (r *Releases) verifyAfter(ctx context.Context, digest fingerprint.Digest, since uint64) (releaseIntegrity, error) {
	for {
		r.mu.Lock()
		if cached, found := r.verified[digest]; found && cached.started >= since {
			r.mu.Unlock()
			return cached, nil
		}
		current, running := r.verifying[digest]
		if !running {
			r.nextVerification++
			current = &verification{done: make(chan struct{}), started: r.nextVerification}
			r.verifying[digest] = current
			r.mu.Unlock()
			return r.lead(ctx, digest, current)
		}
		r.mu.Unlock()
		select {
		case <-current.done:
		case <-ctx.Done():
			return releaseIntegrity{}, context.Cause(ctx)
		}
		// A scan that ended with its own caller's cancellation says nothing
		// about the bytes; this caller scans under its own context instead.
		canceled := errors.Is(current.err, context.Canceled) || errors.Is(current.err, context.DeadlineExceeded)
		if current.started >= since && !canceled {
			return current.integrity, current.err
		}
	}
}

// lead runs the scan of current and settles its outcome, the cache entry and
// the end of the flight in one critical section, so no caller can find the
// flight over without also finding its outcome.
func (r *Releases) lead(ctx context.Context, digest fingerprint.Digest, current *verification) (releaseIntegrity, error) {
	integrity, err := r.inspect(ctx, digest)
	r.mu.Lock()
	defer r.mu.Unlock()
	if err == nil {
		integrity.started = current.started
		integrity = r.commit(digest, integrity)
	} else if cached, found := r.verified[digest]; found && ctx.Err() == nil && cached.verifiedAt < current.started {
		delete(r.verified, digest)
	}
	current.integrity, current.err = integrity, err
	delete(r.verifying, digest)
	close(current.done)
	return integrity, err
}

func (r *Releases) inspect(ctx context.Context, digest fingerprint.Digest) (releaseIntegrity, error) {
	dir, err := r.Root(digest)
	if err != nil {
		return releaseIntegrity{}, err
	}
	return verifyTree(ctx, dir, digest)
}

func (r *Releases) integrity(ctx context.Context, digest fingerprint.Digest) (releaseIntegrity, error) {
	if err := ctx.Err(); err != nil {
		return releaseIntegrity{}, err
	}
	integrity, err := r.verifyAfter(ctx, digest, 0)
	if err != nil {
		return releaseIntegrity{}, errors.Join(plugin.ErrUnavailable, err)
	}
	return integrity, nil
}

// verifiedRoot opens a release whose bytes were verified at some point and
// whose directory is still the one verified. Readers that check each file they
// consume against the content index, or that only need declared paths, use it.
func (r *Releases) verifiedRoot(ctx context.Context, digest fingerprint.Digest) (*os.Root, releaseIntegrity, error) {
	integrity, err := r.integrity(ctx, digest)
	if err != nil {
		return nil, releaseIntegrity{}, err
	}
	root, err := r.openRoot(digest, integrity)
	if err != nil {
		return nil, releaseIntegrity{}, err
	}
	return root, integrity, nil
}

// currentRoot opens a release whose bytes must match its digest now. The cached
// proof is reused only while every entry keeps the settled change stamp it had
// when it was read. Otherwise the caller takes a proof from a scan that began
// after it saw the change, and still requires that proof to match the tree it
// opens: a proof that no longer matches is withdrawn, never accepted.
func (r *Releases) currentRoot(ctx context.Context, digest fingerprint.Digest) (*os.Root, error) {
	var since uint64
	if integrity, found := r.cached(digest); found {
		if root, err := r.openRoot(digest, integrity); err == nil {
			reusable, err := integrity.reusable(ctx, root)
			if err == nil && reusable {
				return root, nil
			}
			if err := errors.Join(err, root.Close()); context.Cause(ctx) != nil {
				return nil, errors.Join(context.Cause(ctx), err)
			}
		}
		since = r.observe()
	}
	// Without an observation the first proof may come from a scan that read
	// the tree before a change; finding it stale is that observation, and
	// only a proof that is stale even after it makes the release unavailable.
	for {
		integrity, err := r.verifyAfter(ctx, digest, since)
		if err != nil {
			return nil, errors.Join(plugin.ErrUnavailable, err)
		}
		root, err := r.openRoot(digest, integrity)
		if err == nil {
			var current bool
			if current, err = integrity.current(ctx, root); err == nil && current {
				return root, nil
			}
			err = errors.Join(err, root.Close())
		}
		if cause := context.Cause(ctx); cause != nil {
			return nil, errors.Join(cause, err)
		}
		r.withdraw(digest, integrity)
		if since != 0 {
			return nil, errors.Join(plugin.ErrUnavailable, errors.New("pluginpackage: release changed after verification"), err)
		}
		since = r.observe()
	}
}

// openRoot keeps the verified directory capability alive for its consumer.
// Reopening its pathname after validation could select a replacement directory.
func (r *Releases) openRoot(digest fingerprint.Digest, integrity releaseIntegrity) (*os.Root, error) {
	dir, err := r.Root(digest)
	if err != nil {
		return nil, err
	}
	entry, err := os.Lstat(dir)
	if err != nil || !entry.IsDir() || !os.SameFile(entry, integrity.directory) {
		return nil, errors.Join(plugin.ErrUnavailable, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, errors.Join(plugin.ErrUnavailable, err)
	}
	info, err := root.Stat(".")
	if err != nil || !os.SameFile(info, integrity.directory) {
		return nil, errors.Join(plugin.ErrUnavailable, err, root.Close())
	}
	return root, nil
}

func verifyTree(ctx context.Context, dir string, digest fingerprint.Digest) (releaseIntegrity, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return releaseIntegrity{}, fmt.Errorf("pluginpackage: resolve release directory: %w", err)
	}
	if resolved != dir {
		return releaseIntegrity{}, fmt.Errorf("%w: release directory is redirected", plugin.ErrInvalid)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return releaseIntegrity{}, fmt.Errorf("pluginpackage: open release directory: %w", err)
	}
	actual, contents, err := treeDigest(ctx, root)
	if err != nil {
		return releaseIntegrity{}, errors.Join(fmt.Errorf("pluginpackage: digest release tree: %w", err), root.Close())
	}
	if actual != digest {
		return releaseIntegrity{}, errors.Join(errors.New("pluginpackage: release integrity mismatch"), root.Close())
	}
	current, err := contents.current(ctx, root)
	if err == nil && !current {
		err = errors.New("pluginpackage: release changed during inspection")
	}
	if err != nil {
		return releaseIntegrity{}, errors.Join(err, root.Close())
	}
	info, err := root.Stat(".")
	if err != nil {
		return releaseIntegrity{}, errors.Join(fmt.Errorf("pluginpackage: inspect release directory: %w", err), root.Close())
	}
	return releaseIntegrity{treeContents: contents, directory: info}, root.Close()
}

// treeDigest is the release content identity. Each entry's stamp is taken
// before its bytes are read, so a change during or after the read moves the
// stamp away from the recorded one, unless the filesystem stamps both within
// one tick of its clock; settled tells those entries apart.
func treeDigest(ctx context.Context, root *os.Root) (fingerprint.Digest, treeContents, error) {
	scanned, stamped, err := changeClock(root)
	if err != nil {
		return fingerprint.Digest{}, treeContents{}, err
	}
	h := sha256.New()
	contents := treeContents{files: map[string][sha256.Size]byte{}, scanned: scanned}
	if stamped {
		contents.stamps = map[string]entryStamp{}
	}
	var total int64
	var count int
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		count++
		if count > MaxFiles {
			return errors.New("pluginpackage: entry limit exceeded")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if contents.stamps != nil {
			if stamp, stamped := stampOf(info); stamped {
				contents.stamps[name] = stamp
			} else {
				contents.stamps = nil
			}
		}
		if entry.IsDir() {
			if !plugin.ValidResourcePath(name) {
				return plugin.ErrInvalid
			}
			fmt.Fprintf(h, "%d:%s:dir:", len(name), name)
			return nil
		}
		if !plugin.ValidResourcePath(name) || !info.Mode().IsRegular() {
			return errors.New("pluginpackage: invalid release entry")
		}
		total += info.Size()
		if total > MaxPackageBytes || info.Size() > MaxFileBytes {
			return errors.New("pluginpackage: release limit exceeded")
		}
		fmt.Fprintf(h, "%d:%s:%o:%d:", len(name), name, info.Mode().Perm()&0111, info.Size())
		f, err := root.Open(name)
		if err != nil {
			return err
		}
		fileHash := sha256.New()
		n, err := io.Copy(io.MultiWriter(h, fileHash), io.LimitReader(f, MaxFileBytes+1))
		err = errors.Join(err, f.Close())
		if n != info.Size() {
			return errors.Join(errors.New("pluginpackage: release changed during inspection"), err)
		}
		if err == nil {
			contents.files[name] = [sha256.Size]byte(fileHash.Sum(nil))
		}
		return err
	})
	if err != nil {
		return fingerprint.Digest{}, treeContents{}, err
	}
	return fingerprint.Sum([sha256.Size]byte(h.Sum(nil))), contents, nil
}

// changeClock reads the filesystem's clock at the granularity it stamps change
// times with, by creating a file beside root: release trees and their staging
// live directly in the Runtime-owned release directory, on one filesystem. It
// reports false, without probing, where entries carry no trusted change time.
func changeClock(root *os.Root) (int64, bool, error) {
	info, err := root.Stat(".")
	if err != nil {
		return 0, false, fmt.Errorf("pluginpackage: inspect release directory: %w", err)
	}
	if _, stamped := stampOf(info); !stamped {
		return 0, false, nil
	}
	probe, err := os.CreateTemp(filepath.Dir(root.Name()), ".clock-")
	if err != nil {
		return 0, false, fmt.Errorf("pluginpackage: read filesystem clock: %w", err)
	}
	info, err = probe.Stat()
	if err = errors.Join(err, probe.Close(), os.Remove(probe.Name())); err != nil {
		return 0, false, fmt.Errorf("pluginpackage: read filesystem clock: %w", err)
	}
	stamp, stamped := stampOf(info)
	return stamp.change, stamped, nil
}

// settled reports whether every entry changed strictly before the scan began.
// An entry the filesystem stamped within the tick the scan started in could be
// rewritten after its read and keep the recorded stamp (git's racy-clean
// entry), so only a later full scan can vouch for it.
func (c treeContents) settled() bool {
	for _, stamp := range c.stamps {
		if stamp.change >= c.scanned {
			return false
		}
	}
	return true
}

// reusable reports whether the proof still vouches for root without reading
// any bytes.
func (c treeContents) reusable(ctx context.Context, root *os.Root) (bool, error) {
	if c.stamps == nil || !c.settled() {
		return false, nil
	}
	return c.matches(ctx, root)
}

// current reports whether a just-completed scan still describes root. Unsettled
// entries are compared by stamp alone: a same-tick rewrite after the read is
// the same race as one after this check. Without stamps the scan itself is the
// only evidence.
func (c treeContents) current(ctx context.Context, root *os.Root) (bool, error) {
	if c.stamps == nil {
		return true, nil
	}
	return c.matches(ctx, root)
}

// matches reports whether every entry of root still carries the stamp it had
// when its bytes were digested, and no entry was added or removed. It reads
// metadata only.
func (c treeContents) matches(ctx context.Context, root *os.Root) (bool, error) {
	errChanged := errors.New("changed")
	seen := 0
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		seen++
		info, err := entry.Info()
		if err != nil {
			return err
		}
		stamp, stamped := stampOf(info)
		if recorded, found := c.stamps[name]; !stamped || !found || stamp != recorded {
			return errChanged
		}
		return nil
	})
	if errors.Is(err, errChanged) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return seen == len(c.stamps), nil
}
