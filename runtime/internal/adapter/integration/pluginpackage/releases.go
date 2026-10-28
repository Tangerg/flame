// Package pluginpackage interprets Flame metadata in neutral Qimu releases.
// Qimu owns archive admission, release identity, integrity and content claims.
package pluginpackage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/qimu"
	"github.com/Tangerg/flame/qimu/cancellation"
	"github.com/Tangerg/flame/qimu/distribution"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/samber/lo"
)

const maxPackageBytes int64 = 128 << 20
const maxManifestBytes int64 = 4 << 20

var errResourceLimit = errors.New("pluginpackage: resource limit exceeded")

type releaseCatalog interface {
	Get(context.Context, flameidentity.Digest) (plugin.Release, error)
	Admit(context.Context, plugin.Release) error
}

type Releases struct {
	content       *distribution.Store
	dataDirectory string
	catalog       releaseCatalog
}

func New(content *distribution.Store, dataDirectory string, catalog releaseCatalog) (*Releases, error) {
	if content == nil || lo.IsNil(catalog) || !filepath.IsAbs(dataDirectory) {
		return nil, errors.New("pluginpackage: content store, absolute data directory and release catalog are required")
	}
	return &Releases{content: content, dataDirectory: filepath.Clean(dataDirectory), catalog: catalog}, nil
}

// Root is a display projection, not a content claim. Reads and process launch
// retain an original Qimu Lease before using this location.
func (r *Releases) Root(digest flameidentity.Digest) (string, error) {
	if err := digest.Validate(); err != nil {
		return "", errors.Join(plugin.ErrInvalid, err)
	}
	return r.content.Path(qimu.Digest(digest.String()))
}

type Candidate struct {
	releases *Releases
	lease    *distribution.Lease
	release  plugin.Release
}

func (c *Candidate) Release() plugin.Release { return c.release }
func (c *Candidate) Publish(ctx context.Context) (plugin.Release, error) {
	if _, err := c.lease.Directory(); err != nil {
		return plugin.Release{}, err
	}
	if err := c.releases.catalog.Admit(ctx, c.release); err != nil {
		return plugin.Release{}, err
	}
	return c.releases.catalog.Get(ctx, c.release.Digest())
}
func (c *Candidate) Discard() error { return c.lease.Close() }

// Materialize holds the neutral release before its bytes become visible. The
// claim survives metadata publication until the installation workflow discards
// this original candidate, including admission failures and response loss.
func (r *Releases) Materialize(ctx context.Context, source string) (_ plugins.Candidate, err error) {
	if err := cancellation.Err(ctx); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("%w: source must be an absolute Runtime path", plugin.ErrInvalid)
	}
	info, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	var lease *distribution.Lease
	if info.IsDir() {
		lease, err = r.content.InstallDirectoryHeld(ctx, source)
	} else if info.Mode().IsRegular() {
		file, _, openErr := filesystem.OpenFile(source, distribution.DefaultLimits().ArchiveBytes)
		if openErr != nil {
			return nil, openErr
		}
		hash := sha256.New()
		_, readErr := io.Copy(hash, cancellation.Reader(ctx, file))
		if err = errors.Join(readErr, file.Close()); err != nil {
			return nil, err
		}
		lease, err = r.content.InstallArchiveHeld(ctx, source, qimu.Digest(hex.EncodeToString(hash.Sum(nil))))
	} else {
		return nil, fmt.Errorf("%w: package source must be a directory or regular archive: %w", plugin.ErrInvalid, filesystem.ErrNotRegular)
	}
	if err != nil {
		return nil, packageInputError(err)
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, lease.Close())
		}
	}()
	neutral := lease.Release()
	digest, err := flameidentity.ParseDigest(neutral.Digest.String())
	if err != nil {
		return nil, err
	}
	directory, err := lease.Directory()
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, root.Close()) }()
	release, err := r.catalog.Get(ctx, digest)
	if errors.Is(err, plugin.ErrNotFound) {
		release, err = parse(ctx, root, digest)
	}
	if err != nil {
		return nil, err
	}
	if release.Name() != neutral.Manifest.Package || release.Declaration().Version != neutral.Manifest.Version {
		return nil, fmt.Errorf("%w: Flame metadata disagrees with its neutral manifest", plugin.ErrInvalid)
	}
	if err := validateProjection(ctx, root, release, neutral.Manifest); err != nil {
		return nil, err
	}
	return &Candidate{releases: r, lease: lease, release: release}, nil
}

// Reclaim collects content only. Immutable metadata remains an interpretation of
// its original release for durable history; it never grants availability.
func (r *Releases) Reclaim(ctx context.Context) error {
	digests, err := r.content.Digests(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, digest := range digests {
		if err := r.content.Remove(ctx, digest); err != nil && !errors.Is(err, qimu.ErrInUse) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (r *Releases) readResource(ctx context.Context, installation *plugin.Installation, name string, limit int64) (_ []byte, err error) {
	if !plugin.ValidResourcePath(name) {
		return nil, fmt.Errorf("%w: invalid resource path", plugin.ErrInvalid)
	}
	lease, err := r.content.Acquire(ctx, qimu.Digest(installation.Selected().String()))
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	content, err := lease.ReadLimit(ctx, name, limit)
	if errors.Is(err, distribution.ErrCapacity) {
		return nil, errors.Join(errResourceLimit, err)
	}
	return content, err
}
func (r *Releases) ReadResource(ctx context.Context, installation *plugin.Installation, name string) ([]byte, error) {
	body, err := r.readResource(ctx, installation, name, maxPackageBytes)
	if err != nil && !errors.Is(err, plugin.ErrInvalid) && cancellation.Err(ctx) == nil {
		return nil, errors.Join(plugin.ErrUnavailable, err)
	}
	return body, err
}

func packageInputError(err error) error {
	if errors.Is(err, distribution.ErrInvalid) || errors.Is(err, distribution.ErrIntegrity) || errors.Is(err, distribution.ErrCapacity) || errors.Is(err, filesystem.ErrFileTooLarge) || errors.Is(err, filesystem.ErrNotRegular) || errors.Is(err, os.ErrNotExist) {
		return errors.Join(plugin.ErrInvalid, err)
	}
	return err
}

// read applies a Flame format budget inside an already acquired content tree.
func read(ctx context.Context, root *os.Root, name string, limit int64) (body []byte, err error) {
	if cause := cancellation.Err(ctx); cause != nil {
		return nil, cause
	}
	file, _, err := filesystem.OpenFileAt(root, name, limit)
	if err != nil {
		if errors.Is(err, filesystem.ErrFileTooLarge) {
			return nil, errors.Join(errResourceLimit, err)
		}
		return nil, err
	}
	defer func() {
		err = errors.Join(err, file.Close())
		if err != nil {
			body = nil
		}
	}()
	body, err = io.ReadAll(io.LimitReader(cancellation.Reader(ctx, file), limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errResourceLimit
	}
	return body, cancellation.Err(ctx)
}
