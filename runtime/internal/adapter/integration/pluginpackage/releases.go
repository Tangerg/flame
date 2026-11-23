package pluginpackage

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/capture"
	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"

	"github.com/Tangerg/frame/distribution"
	framehost "github.com/Tangerg/frame/host"
	framevalue "github.com/Tangerg/frame/value"
	"github.com/samber/lo"
)

const maxPackageBytes int64 = 128 << 20
const maxManifestBytes int64 = 4 << 20

const (
	pluginManifestFile = "plugin.json"
	mcpManifestFile    = "mcp.json"
)

var errResourceLimit = errors.New("pluginpackage: resource limit exceeded")

type releaseCatalog interface {
	Get(context.Context, framevalue.Digest) (plugin.Release, error)
	Admit(context.Context, plugin.Release) error
}

type Releases struct {
	content       distribution.Store
	dataDirectory string
	catalog       releaseCatalog
}

func New(content distribution.Store, dataDirectory string, catalog releaseCatalog) (*Releases, error) {
	if !content.Valid() || lo.IsNil(catalog) || !filepath.IsAbs(dataDirectory) {
		return nil, errors.New("pluginpackage: content store, absolute data directory and release catalog are required")
	}
	for len(dataDirectory) > 0 && os.IsPathSeparator(dataDirectory[len(dataDirectory)-1]) {
		dataDirectory = dataDirectory[:len(dataDirectory)-1]
	}
	parentPath, namespace := filepath.Split(dataDirectory)
	if namespace == "" || namespace == "." || namespace == ".." {
		return nil, errors.New("pluginpackage: data directory requires a named namespace")
	}
	// Cleaning before resolution changes symlink/.. traversal; resolving the
	// namespace itself would grant authority through its redirection.
	parent, err := filesystem.CanonicalPath(parentPath)
	if err != nil {
		return nil, fmt.Errorf("pluginpackage: resolve data directory parent: %w", err)
	}
	return &Releases{content: content, dataDirectory: filepath.Join(parent, namespace), catalog: catalog}, nil
}

func (r *Releases) Root(digest framevalue.Digest) (string, error) {
	if err := digest.Validate(); err != nil {
		return "", errors.Join(plugin.ErrInvalid, err)
	}
	return r.content.Path(digest)
}

type Candidate struct {
	releases *Releases
	lease    distribution.Lease
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

func (r *Releases) importSource(ctx context.Context, source string, info os.FileInfo) (distribution.Lease, error) {
	var lease distribution.Lease
	var err error
	switch {
	case info.IsDir():
		lease, err = r.content.ImportDirectory(ctx, source)
	case info.Mode().IsRegular():
		var digest framevalue.Digest
		digest, err = localArchiveDigest(ctx, source, info)
		if err == nil {
			lease, err = r.content.ImportArchive(ctx, source, digest)
		}
	default:
		return lease, fmt.Errorf("%w: package source must be a directory or regular archive: %w", plugin.ErrInvalid, filesystem.ErrNotRegular)
	}
	if err != nil {
		return lease, packageInputError(err)
	}
	return lease, nil
}

func (r *Releases) Materialize(ctx context.Context, source string) (_ plugins.Candidate, err error) {
	if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(source) {
		return nil, fmt.Errorf("%w: source must be an absolute Runtime path", plugin.ErrInvalid)
	}
	info, err := os.Lstat(source)
	if err != nil {
		return nil, err
	}
	lease, err := r.importSource(ctx, source, info)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, lease.Close())
		}
	}()
	neutral := lease.Snapshot()
	digest := neutral.Digest
	if err := digest.Validate(); err != nil {
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

func (r *Releases) Reclaim(ctx context.Context) error {
	digests, err := r.content.Digests(ctx)
	if err != nil {
		return err
	}
	var failures []error
	for _, digest := range digests {
		if err := r.content.Remove(ctx, digest); err != nil && !errors.Is(err, framehost.ErrInUse) {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (r *Releases) readResource(ctx context.Context, installation plugin.Installation, name string, limit int64) (_ []byte, err error) {
	if !plugin.ValidResourcePath(name) {
		return nil, fmt.Errorf("%w: invalid resource path", plugin.ErrInvalid)
	}
	lease, err := r.content.Acquire(ctx, installation.Selected())
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, lease.Close()) }()
	for _, artifact := range lease.Snapshot().Manifest.Artifacts {
		if artifact.Path == name && artifact.Size > limit {
			return nil, errResourceLimit
		}
	}
	content, err := lease.Read(ctx, name)
	if errors.Is(err, distribution.ErrCapacity) {
		return nil, errors.Join(errResourceLimit, err)
	}
	return content, err
}

func (r *Releases) ReadResource(ctx context.Context, installation plugin.Installation, name string) ([]byte, error) {
	body, err := r.readResource(ctx, installation, name, maxPackageBytes)
	if err != nil && !errors.Is(err, plugin.ErrInvalid) && errors.Join(ctx.Err(), context.Cause(ctx)) == nil {
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

func read(ctx context.Context, root *os.Root, name string, limit int64) (body []byte, err error) {
	if cause := errors.Join(ctx.Err(), context.Cause(ctx)); cause != nil {
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
	contextReader, err := capture.NewReader(ctx, file)
	if err != nil {
		return nil, err
	}
	body, err = io.ReadAll(io.LimitReader(contextReader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, errResourceLimit
	}
	return body, errors.Join(ctx.Err(), context.Cause(ctx))
}

func localArchiveDigest(ctx context.Context, path string, expected os.FileInfo) (_ framevalue.Digest, err error) {
	file, opened, err := filesystem.OpenFileExpected(path, expected, distribution.DefaultLimits().MaxArchiveBytes)
	if err != nil {
		return framevalue.Digest{}, err
	}
	defer func() { err = errors.Join(err, file.Close()) }()
	reader, err := capture.NewReader(ctx, file)
	if err != nil {
		return framevalue.Digest{}, err
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, io.LimitReader(reader, distribution.DefaultLimits().MaxArchiveBytes+1)); err != nil {
		return framevalue.Digest{}, err
	}
	if err := filesystem.VerifyPathVersion(file, opened, path); err != nil {
		return framevalue.Digest{}, err
	}
	return framevalue.DigestOf([sha256.Size]byte(hash.Sum(nil))), nil
}
