// Package pluginpackage translates portable packages and owns confined immutable
// release bytes. Inspection never runs package code.
package pluginpackage

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"golang.org/x/text/unicode/norm"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
)

const MaxPackageBytes int64 = 128 << 20
const MaxFileBytes int64 = 16 << 20
const MaxManifestBytes int64 = 4 << 20

var errResourceLimit = errors.New("pluginpackage: resource limit exceeded")

const MaxFiles = 4096
const maxVerifiedReleases = 2 * plugin.MaxInstallations

type releaseCatalog interface {
	Get(context.Context, string) (plugin.Release, error)
	Admit(context.Context, plugin.Release) error
}
type releaseIntegrity struct {
	files      map[string][sha256.Size]byte
	directory  fs.FileInfo
	verifiedAt uint64
}
type Releases struct {
	directory        string
	catalog          releaseCatalog
	publishMu        sync.Mutex
	loadMu           sync.Mutex
	mu               sync.Mutex
	verified         map[string]releaseIntegrity
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
	var err error
	if err = os.MkdirAll(absolute, 0700); err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	return &Releases{directory: resolved, catalog: catalog, verified: map[string]releaseIntegrity{}}, nil
}

// The cache retains only byte integrity, never another interpretation of an
// admitted declaration. Cold eviction rechecks the package's content identity.
func (r *Releases) remember(digest string, integrity releaseIntegrity) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.verified[digest]; !exists && len(r.verified) == maxVerifiedReleases {
		var oldest string
		for key, value := range r.verified {
			if oldest == "" || value.verifiedAt < r.verified[oldest].verifiedAt {
				oldest = key
			}
		}
		delete(r.verified, oldest)
	}
	r.nextVerification++
	integrity.verifiedAt = r.nextVerification
	r.verified[digest] = integrity
}
func (r *Releases) Root(digest string) (string, error) {
	if !plugin.ValidDigest(digest) {
		return "", plugin.ErrInvalid
	}
	return filepath.Join(r.directory, digest), nil
}
func (r *Releases) Materialize(ctx context.Context, source string) (release plugin.Release, err error) {
	if !filepath.IsAbs(source) {
		return release, fmt.Errorf("%w: source must be an absolute Runtime path", plugin.ErrInvalid)
	}
	staging, err := os.MkdirTemp(r.directory, ".stage-")
	if err != nil {
		return release, err
	}
	defer func() {
		if _, exists := os.Lstat(staging); errors.Is(exists, fs.ErrNotExist) {
			return
		}
		restoreErr := filepath.WalkDir(staging, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return os.Chmod(name, 0700)
			}
			return nil
		})
		err = errors.Join(err, restoreErr, os.RemoveAll(staging))
	}()
	target, err := os.OpenRoot(staging)
	if err != nil {
		return release, err
	}
	defer func() { err = errors.Join(err, target.Close()) }()
	var copied int64
	seen := map[string]string{}
	entries := map[string]bool{}
	admit := func(name string, isDir bool) error {
		if !plugin.ValidResourcePath(name) {
			return fmt.Errorf("%w: invalid package entry", plugin.ErrInvalid)
		}
		if entries[name] {
			return fmt.Errorf("%w: duplicate package entry", plugin.ErrInvalid)
		}
		entries[name] = true
		for segment := name; segment != "."; segment = path.Dir(segment) {
			key := strings.ToLower(norm.NFC.String(segment))
			if prior, found := seen[key]; found && prior != segment {
				return fmt.Errorf("%w: ambiguous package destination", plugin.ErrInvalid)
			}
			seen[key] = segment
		}
		if len(seen) > MaxFiles {
			return fmt.Errorf("%w: entry limit exceeded", plugin.ErrInvalid)
		}
		if isDir {
			return target.MkdirAll(name, 0700)
		}
		return nil
	}
	write := func(name string, mode fs.FileMode, input io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if mode&fs.ModeType != 0 {
			return fmt.Errorf("%w: invalid package entry mode", plugin.ErrInvalid)
		}
		if err := admit(name, false); err != nil {
			return err
		}
		if err := target.MkdirAll(path.Dir(name), 0700); err != nil {
			return err
		}
		f, err := target.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600|mode.Perm()&0111)
		if err != nil {
			return err
		}
		limit := min(MaxFileBytes, MaxPackageBytes-copied)
		n, copyErr := io.Copy(f, io.LimitReader(input, limit+1))
		closeErr := f.Close()
		copied += n
		if n > limit {
			return errors.Join(fmt.Errorf("%w: byte limit exceeded", plugin.ErrInvalid), copyErr, closeErr)
		}
		return errors.Join(copyErr, closeErr)
	}
	info, err := os.Stat(source)
	if err != nil {
		return release, err
	}
	if info.IsDir() {
		input, err := os.OpenRoot(source)
		if err != nil {
			return release, err
		}
		defer func() { err = errors.Join(err, input.Close()) }()
		err = fs.WalkDir(input.FS(), ".", func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				if name == "." {
					return nil
				}
				return admit(name, true)
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%w: package must contain only directories and regular files", plugin.ErrInvalid)
			}
			f, err := input.Open(name)
			if err != nil {
				return err
			}
			writeErr := write(name, info.Mode(), f)
			return errors.Join(writeErr, f.Close())
		})
	} else {
		if info.Size() > MaxPackageBytes {
			return release, fmt.Errorf("%w: archive byte limit exceeded", plugin.ErrInvalid)
		}
		archive, openErr := zip.OpenReader(source)
		if openErr != nil {
			if errors.Is(openErr, zip.ErrFormat) {
				return release, errors.Join(plugin.ErrInvalid, openErr)
			}
			return release, openErr
		}
		for _, entry := range archive.File {
			if entry.FileInfo().IsDir() {
				if entry.Mode()&fs.ModeType != fs.ModeDir {
					err = fmt.Errorf("%w: invalid directory mode", plugin.ErrInvalid)
					break
				}
				err = admit(strings.TrimSuffix(entry.Name, "/"), true)
				if err != nil {
					break
				}
				continue
			}
			f, openErr := entry.Open()
			if openErr != nil {
				err = openErr
				break
			}
			err = errors.Join(write(entry.Name, entry.Mode(), f), f.Close())
			if err != nil {
				break
			}
		}
		err = errors.Join(err, archive.Close())
		if errors.Is(err, zip.ErrFormat) || errors.Is(err, zip.ErrChecksum) || errors.Is(err, zip.ErrAlgorithm) {
			err = errors.Join(plugin.ErrInvalid, err)
		}
	}
	if err != nil {
		return release, err
	}
	digest, files, err := treeDigest(ctx, target)
	if err != nil {
		return release, err
	}
	release, err = r.catalog.Get(ctx, digest)
	if errors.Is(err, plugin.ErrNotFound) {
		release, err = parse(ctx, target, digest)
	}
	if err != nil {
		return release, err
	}
	destination, _ := r.Root(digest)
	// Publication alone serializes the existence check and immutable rename.
	// Independent admissions may copy bytes concurrently, but share one release.
	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	if err = ctx.Err(); err != nil {
		return release, err
	}
	if _, err = os.Stat(destination); err == nil {
		root, _, inspectErr := r.inspectRoot(ctx, digest)
		if inspectErr != nil {
			return release, inspectErr
		}
		if err = root.Close(); err != nil {
			return release, err
		}
		if err = r.catalog.Admit(ctx, release); err != nil {
			return release, err
		}
		return r.catalog.Get(ctx, digest)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return release, err
	}
	if err = fs.WalkDir(target.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		mode := fs.FileMode(0400)
		if entry.IsDir() {
			mode = 0500
		} else {
			info, e := entry.Info()
			if e != nil {
				return e
			}
			mode |= info.Mode().Perm() & 0111
		}
		return target.Chmod(name, mode)
	}); err != nil {
		return release, err
	}
	if err = os.Rename(staging, destination); err != nil {
		return release, err
	}
	if err = r.catalog.Admit(ctx, release); err != nil {
		return release, err
	}
	info, err = target.Stat(".")
	if err != nil {
		return release, err
	}
	r.remember(digest, releaseIntegrity{files: files, directory: info})
	return r.catalog.Get(ctx, digest)
}

func (r *Releases) integrity(ctx context.Context, digest string) (releaseIntegrity, error) {
	if err := ctx.Err(); err != nil {
		return releaseIntegrity{}, err
	}
	r.mu.Lock()
	integrity, found := r.verified[digest]
	r.mu.Unlock()
	if found {
		return integrity, nil
	}
	r.loadMu.Lock()
	defer r.loadMu.Unlock()
	r.mu.Lock()
	integrity, found = r.verified[digest]
	r.mu.Unlock()
	if found {
		return integrity, nil
	}
	root, integrity, err := r.inspectRoot(ctx, digest)
	if err != nil {
		return releaseIntegrity{}, errors.Join(plugin.ErrUnavailable, err)
	}
	if err := root.Close(); err != nil {
		return releaseIntegrity{}, err
	}
	return integrity, nil
}
func (r *Releases) verifiedRoot(ctx context.Context, digest string) (*os.Root, releaseIntegrity, error) {
	integrity, err := r.integrity(ctx, digest)
	if err != nil {
		return nil, releaseIntegrity{}, err
	}
	dir, err := r.Root(digest)
	if err != nil {
		return nil, releaseIntegrity{}, err
	}
	entry, err := os.Lstat(dir)
	if err != nil || !entry.IsDir() || !os.SameFile(entry, integrity.directory) {
		return nil, releaseIntegrity{}, errors.Join(plugin.ErrUnavailable, err)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, releaseIntegrity{}, errors.Join(plugin.ErrUnavailable, err)
	}
	info, err := root.Stat(".")
	if err != nil || !os.SameFile(info, integrity.directory) {
		return nil, releaseIntegrity{}, errors.Join(plugin.ErrUnavailable, err, root.Close())
	}
	return root, integrity, nil
}

// inspectRoot keeps the verified directory capability alive for its consumer.
// Reopening its pathname after validation could select a replacement directory.
func (r *Releases) inspectRoot(ctx context.Context, digest string) (*os.Root, releaseIntegrity, error) {
	dir, err := r.Root(digest)
	if err != nil {
		return nil, releaseIntegrity{}, err
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, releaseIntegrity{}, err
	}
	if resolved != dir {
		return nil, releaseIntegrity{}, plugin.ErrInvalid
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, releaseIntegrity{}, err
	}
	actual, files, err := treeDigest(ctx, root)
	if err != nil {
		return nil, releaseIntegrity{}, errors.Join(err, root.Close())
	}
	if actual != digest {
		return nil, releaseIntegrity{}, errors.Join(errors.New("pluginpackage: release integrity mismatch"), root.Close())
	}
	info, err := root.Stat(".")
	if err != nil {
		return nil, releaseIntegrity{}, errors.Join(err, root.Close())
	}
	integrity := releaseIntegrity{files: files, directory: info}
	r.remember(digest, integrity)
	return root, integrity, nil
}
func (r *Releases) readResource(ctx context.Context, installation *plugin.Installation, name string, limit int64) ([]byte, error) {
	if !plugin.ValidResourcePath(name) {
		return nil, fmt.Errorf("%w: invalid resource path", plugin.ErrInvalid)
	}
	root, integrity, err := r.verifiedRoot(ctx, installation.Snapshot().Selected.Digest)
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
func treeDigest(ctx context.Context, root *os.Root) (string, map[string][sha256.Size]byte, error) {
	h := sha256.New()
	files := map[string][sha256.Size]byte{}
	var total int64
	var count int
	err := fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
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
		if entry.IsDir() {
			if !plugin.ValidResourcePath(name) {
				return plugin.ErrInvalid
			}
			fmt.Fprintf(h, "%d:%s:dir:", len(name), name)
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !plugin.ValidResourcePath(name) || !info.Mode().IsRegular() {
			return errors.New("pluginpackage: invalid release entry")
		}
		total += info.Size()
		if count > MaxFiles || total > MaxPackageBytes || info.Size() > MaxFileBytes {
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
			files[name] = [sha256.Size]byte(fileHash.Sum(nil))
		}
		return err
	})
	if err != nil {
		return "", nil, err
	}
	return hex.EncodeToString(h.Sum(nil)), files, nil
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
func expand(value, root, data string) string {
	return strings.NewReplacer("${PLUGIN_ROOT}", root, "${PLUGIN_DATA}", data).Replace(value)
}
