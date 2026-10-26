package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

var errDataDirectoryInUse = errors.New("runtime data directory is already in use")

// dataDirectory owns the physical directory until every Runtime resource closes.
type dataDirectory struct {
	directory string
	lease     *filesystem.DirectoryLease
	identity  os.FileInfo
}

func openDataDirectory(ctx context.Context, directory string) (*dataDirectory, error) {
	if ctx == nil {
		return nil, errors.New("runtime ownership: context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if directory == "" {
		return nil, errors.New("runtime ownership: data directory is required")
	}
	if !filepath.IsAbs(directory) {
		return nil, errors.New("runtime ownership: data directory must be absolute")
	}
	cleaned := filepath.Clean(directory)
	if err := os.MkdirAll(cleaned, 0o700); err != nil {
		return nil, fmt.Errorf("runtime ownership: create data directory %q: %w", cleaned, err)
	}
	canonical, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return nil, fmt.Errorf("runtime ownership: resolve data directory %q: %w", cleaned, err)
	}
	canonical = filepath.Clean(canonical)
	identity, err := os.Stat(canonical)
	if err != nil {
		return nil, err
	}
	lease, err := filesystem.TryDirectoryLease(canonical)
	if err != nil {
		return nil, fmt.Errorf("runtime ownership: acquire data directory %q: %w", canonical, err)
	}
	if lease == nil {
		return nil, fmt.Errorf("%w: %q", errDataDirectoryInUse, canonical)
	}
	opened, err := os.Stat(canonical)
	if err != nil || !os.SameFile(identity, opened) {
		return nil, errors.Join(errors.New("runtime ownership: data directory changed during acquisition"), err, lease.Release())
	}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, lease.Release())
	}
	if err := os.Chmod(canonical, 0o700); err != nil {
		return nil, errors.Join(fmt.Errorf("runtime ownership: protect data directory %q: %w", canonical, err), lease.Release())
	}
	return &dataDirectory{directory: canonical, lease: lease, identity: identity}, nil
}

func (d *dataDirectory) validate() error {
	current, err := os.Stat(d.directory)
	if err != nil {
		return fmt.Errorf("runtime ownership: inspect data directory: %w", err)
	}
	if !os.SameFile(d.identity, current) {
		return errors.New("runtime ownership: data directory changed while Runtime owns it")
	}
	return nil
}

func (d *dataDirectory) Close() error {
	if err := d.lease.Release(); err != nil {
		return fmt.Errorf("runtime ownership: release data directory %q: %w", d.directory, err)
	}
	return nil
}
