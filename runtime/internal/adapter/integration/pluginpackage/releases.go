// Package pluginpackage translates portable packages and owns confined immutable
// release bytes. Inspection never runs package code.
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
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
)

const MaxPackageBytes int64 = 128 << 20
const MaxFileBytes int64 = 16 << 20
const MaxManifestBytes int64 = 4 << 20

var errResourceLimit = errors.New("pluginpackage: resource limit exceeded")

const MaxFiles = 4096

type releaseCatalog interface {
	Get(context.Context, fingerprint.Digest) (plugin.Release, error)
	Admit(context.Context, plugin.Release) error
	Digests(context.Context) ([]fingerprint.Digest, error)
	Remove(context.Context, fingerprint.Digest) error
}

type Releases struct {
	directory string
	catalog   releaseCatalog
	// publishMu serializes publication and reclamation of release directories.
	publishMu sync.Mutex
	// mu guards the integrity cache and the verifications in flight. It is
	// never held across filesystem I/O.
	mu               sync.Mutex
	verified         map[fingerprint.Digest]releaseIntegrity
	verifying        map[fingerprint.Digest]*verification
	nextVerification uint64
}

func New(directory string, catalog releaseCatalog) (*Releases, error) {
	if catalog == nil {
		return nil, errors.New("pluginpackage: release catalog is required")
	}
	if !filepath.IsAbs(directory) {
		return nil, errors.New("pluginpackage: release directory must be absolute")
	}
	absolute := filepath.Clean(directory)
	if err := os.MkdirAll(absolute, 0700); err != nil {
		return nil, fmt.Errorf("pluginpackage: create release directory: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("pluginpackage: resolve release directory: %w", err)
	}
	return &Releases{
		directory: resolved,
		catalog:   catalog,
		verified:  map[fingerprint.Digest]releaseIntegrity{},
		verifying: map[fingerprint.Digest]*verification{},
	}, nil
}

func (r *Releases) Root(digest fingerprint.Digest) (string, error) {
	if err := digest.Validate(); err != nil {
		return "", errors.Join(plugin.ErrInvalid, err)
	}
	return filepath.Join(r.directory, digest.String()), nil
}

// Candidate is one package admitted as an immutable release that is not yet
// published: its bytes are sealed in private staging and its declaration is
// interpreted. Publishing it names it in the catalog; discarding it removes
// staging that was never published.
type Candidate struct {
	releases *Releases
	staged   *staging
	release  plugin.Release
	contents treeContents
}

func (c *Candidate) Release() plugin.Release { return c.release }

func (c *Candidate) Publish(ctx context.Context) (plugin.Release, error) {
	return c.releases.publish(ctx, c.staged, c.release, c.contents)
}

func (c *Candidate) Discard() error { return c.staged.discard() }

// Materialize admits one package as a candidate release: the bytes are copied
// into private staging within the package limits, sealed read-only, digested
// and interpreted once per digest. Publication is left to the installation
// change that references the release, so nothing is published outside the
// admission point that also reclaims releases.
func (r *Releases) Materialize(ctx context.Context, source string) (_ plugins.Candidate, err error) {
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("%w: source must be an absolute Runtime path", plugin.ErrInvalid)
	}
	staged, err := newStaging(r.directory)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, staged.discard())
		}
	}()
	if err := staged.extract(ctx, source); err != nil {
		return nil, err
	}
	if err := staged.seal(); err != nil {
		return nil, err
	}
	digest, contents, err := treeDigest(ctx, staged.root)
	if err != nil {
		return nil, err
	}
	release, err := r.admission(ctx, staged.root, digest)
	if err != nil {
		return nil, err
	}
	return &Candidate{releases: r, staged: staged, release: release, contents: contents}, nil
}

// admission returns the catalog's interpretation of a digest it has already
// admitted, so equal bytes are never reinterpreted; only a new digest is parsed.
func (r *Releases) admission(ctx context.Context, root *os.Root, digest fingerprint.Digest) (plugin.Release, error) {
	release, err := r.catalog.Get(ctx, digest)
	if errors.Is(err, plugin.ErrNotFound) {
		return parse(ctx, root, digest)
	}
	return release, err
}

// publish makes sealed staging the release directory for its digest, or
// verifies the directory an earlier admission already published, and admits
// the release to the catalog. publishMu serializes the existence check, the
// rename and reclamation, so a directory is never removed while published.
func (r *Releases) publish(ctx context.Context, staged *staging, release plugin.Release, contents treeContents) (plugin.Release, error) {
	digest := release.Digest()
	destination, err := r.Root(digest)
	if err != nil {
		return plugin.Release{}, err
	}
	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	if err := ctx.Err(); err != nil {
		return plugin.Release{}, err
	}
	if _, err := os.Stat(destination); err == nil {
		if _, err := r.verify(ctx, digest); err != nil {
			return plugin.Release{}, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return plugin.Release{}, err
	} else {
		if err := os.Rename(staged.path, destination); err != nil {
			return plugin.Release{}, err
		}
		info, err := staged.root.Stat(".")
		if err != nil {
			return plugin.Release{}, err
		}
		r.remember(digest, releaseIntegrity{treeContents: contents, directory: info})
	}
	if err := r.catalog.Admit(ctx, release); err != nil {
		return plugin.Release{}, err
	}
	return r.catalog.Get(ctx, digest)
}

// Reclaim removes every admitted release and release directory outside
// retained. The directory goes first: a release whose directory could not be
// removed stays in the catalog and is retried by the next reclamation, and a
// directory without a catalog row is still found by its digest name. Staging
// and entries that do not name a digest are not releases and are left alone.
// The directory is read in bounded batches, so garbage of any size is
// reclaimed without materializing the whole listing.
func (r *Releases) Reclaim(ctx context.Context, retained []fingerprint.Digest) error {
	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	admitted, err := r.catalog.Digests(ctx)
	if err != nil {
		return fmt.Errorf("pluginpackage: list admitted releases: %w", err)
	}
	directory, err := os.Open(r.directory)
	if err != nil {
		return fmt.Errorf("pluginpackage: open release directories: %w", err)
	}
	defer directory.Close()
	var failures error
	reclaim := func(digest fingerprint.Digest) error {
		if slices.Contains(retained, digest) {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		dir, err := r.Root(digest)
		if err != nil {
			return err
		}
		r.forget(digest)
		if err := removeSealed(dir); err != nil {
			failures = errors.Join(failures, fmt.Errorf("pluginpackage: remove release %s directory: %w", digest, err))
			return nil
		}
		if slices.Contains(admitted, digest) {
			if err := r.catalog.Remove(ctx, digest); err != nil {
				failures = errors.Join(failures, fmt.Errorf("pluginpackage: remove release %s: %w", digest, err))
			}
		}
		return nil
	}
	published := make(map[fingerprint.Digest]struct{})
	for {
		entries, readErr := directory.ReadDir(reclaimBatch)
		for _, entry := range entries {
			digest, err := fingerprint.ParseDigest(entry.Name())
			if err != nil {
				continue
			}
			published[digest] = struct{}{}
			if err := reclaim(digest); err != nil {
				return errors.Join(failures, err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return errors.Join(failures, fmt.Errorf("pluginpackage: list release directories: %w", readErr))
		}
	}
	for _, digest := range admitted {
		if _, seen := published[digest]; seen {
			continue
		}
		if err := reclaim(digest); err != nil {
			return errors.Join(failures, err)
		}
	}
	return failures
}

// reclaimBatch bounds one directory read during reclamation.
const reclaimBatch = 256

func (r *Releases) readResource(ctx context.Context, installation *plugin.Installation, name string, limit int64) ([]byte, error) {
	if !plugin.ValidResourcePath(name) {
		return nil, fmt.Errorf("%w: invalid resource path", plugin.ErrInvalid)
	}
	root, integrity, err := r.verifiedRoot(ctx, installation.Selected())
	if err != nil {
		return nil, err
	}
	content, err := read(ctx, root, name, limit)
	if err == nil {
		expected, exists := integrity.files[name]
		if !exists || sha256.Sum256(content) != expected {
			err = plugin.ErrUnavailable
		}
	}
	if err != nil {
		content = nil
	}
	return content, errors.Join(err, root.Close())
}

func read(ctx context.Context, root *os.Root, name string, limit int64) (body []byte, err error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, f.Close())
		if err != nil {
			body = nil
		}
	}()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("pluginpackage: resource must be a regular file")
	}
	if info.Size() > limit {
		return nil, errResourceLimit
	}
	body, err = io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errResourceLimit
	}
	return body, ctx.Err()
}
