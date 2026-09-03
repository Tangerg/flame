// Package statefile owns rooted, atomic file operations for CLI-local state.
package statefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pathologize"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/fileinput"
)

// Store confines named state records to one absolute directory. The value is
// immutable after construction and is safe for concurrent use.
type Store struct {
	root string
}

// Open validates and creates an owned state root.
func Open(root string) (Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return Store{}, errors.New("state directory is empty")
	}
	if !filepath.IsAbs(root) {
		return Store{}, errors.New("state directory must be absolute")
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return Store{}, fmt.Errorf("create state directory: %w", err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return Store{}, fmt.Errorf("resolve state directory: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Store{}, fmt.Errorf("inspect state directory: %w", err)
	}
	if !info.IsDir() {
		return Store{}, errors.New("state root is not a directory")
	}
	return Store{root: root}, nil
}

// Read returns one regular state file within the requested size bound.
func (s Store) Read(name string, maximumBytes int64) ([]byte, error) {
	if maximumBytes <= 0 {
		return nil, errors.New("state read limit must be positive")
	}
	path, err := s.path(name)
	if err != nil {
		return nil, err
	}
	if _, err := s.directory(filepath.Dir(name), false); err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("state file %q is not regular", name)
	}
	if info.Size() > maximumBytes {
		return nil, fmt.Errorf("state file %q exceeds %d bytes", name, maximumBytes)
	}
	file, _, err := fileinput.OpenExpected(path, info, maximumBytes)
	if err != nil {
		switch {
		case errors.Is(err, fileinput.ErrChanged):
			return nil, fmt.Errorf("state file %q changed while it was being opened", name)
		case errors.Is(err, fileinput.ErrNotRegular):
			return nil, fmt.Errorf("state file %q is not regular", name)
		case errors.Is(err, fileinput.ErrTooLarge):
			return nil, fmt.Errorf("state file %q exceeds %d bytes", name, maximumBytes)
		default:
			return nil, err
		}
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(io.LimitReader(file, maximumBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read state file %q: %w", name, err)
	}
	if int64(len(body)) > maximumBytes {
		return nil, fmt.Errorf("state file %q exceeds %d bytes", name, maximumBytes)
	}
	return body, nil
}

// List returns child names in the order supplied by os.ReadDir.
func (s Store) List(directory string) ([]string, error) {
	path, err := s.directory(directory, false)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// Replace atomically publishes one complete state file. The temporary file is
// created beside the destination, so os.Rename cannot cross filesystems and
// intentionally replaces the prior snapshot instead of conflict-renaming it.
func (s Store) Replace(name string, body []byte) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	directory, err := s.directory(filepath.Dir(name), true)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".flame-state-*")
	if err != nil {
		return fmt.Errorf("create state snapshot: %w", err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write state snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync state snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close state snapshot: %w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("replace state snapshot: %w", err)
	}
	removeTemporary = false
	// Rename is the logical commit point. A directory-sync refusal cannot be
	// returned as an apparent failed replacement after the new state is visible,
	// because callers would retain an older in-memory fact or retry a committed
	// mutation. Strengthen crash durability where the filesystem supports it.
	syncCommittedDirectory(directory)
	return nil
}

// Remove retires one state file. Missing files are already removed.
func (s Store) Remove(name string) error {
	path, err := s.path(name)
	if err != nil {
		return err
	}
	directory, err := s.directory(filepath.Dir(name), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	syncCommittedDirectory(directory)
	return nil
}

func (s Store) path(name string) (string, error) {
	if s.root == "" {
		return "", errors.New("state store is not configured")
	}
	if name == "" || name == "." || filepath.IsAbs(name) || filepath.Clean(name) != name || strings.Contains(name, `\`) {
		return "", errors.New("state name must be a clean relative path")
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || !pathologize.IsClean(part) {
			return "", errors.New("state name contains a non-portable path segment")
		}
	}
	return pathologize.Join(s.root, parts...), nil
}

func (s Store) directory(name string, create bool) (string, error) {
	if name == "." {
		return s.root, nil
	}
	destination, err := s.path(name)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(s.root, destination)
	if err != nil {
		return "", fmt.Errorf("resolve state directory: %w", err)
	}
	current := s.root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		parent := current
		current = filepath.Join(current, part)
		info, inspectErr := os.Lstat(current)
		created := false
		if errors.Is(inspectErr, os.ErrNotExist) && create {
			created = true
			if mkdirErr := os.Mkdir(current, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return "", fmt.Errorf("create state directory: %w", mkdirErr)
			}
			info, inspectErr = os.Lstat(current)
		}
		if inspectErr != nil {
			return "", inspectErr
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("state path %q contains a component that is not a directory", name)
		}
		if created {
			if syncErr := syncDirectory(parent); syncErr != nil {
				return "", fmt.Errorf("commit state directory: %w", syncErr)
			}
		}
	}
	return destination, nil
}

func syncDirectory(path string) error {
	directory, _, err := fileinput.OpenDirectory(path)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func syncCommittedDirectory(path string) {
	_ = syncDirectory(path)
}
