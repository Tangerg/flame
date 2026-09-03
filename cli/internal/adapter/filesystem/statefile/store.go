// Package statefile owns rooted, atomic file operations for CLI-local state.
package statefile

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"

	"github.com/spf13/pathologize"

	"github.com/Tangerg/flame/cli/internal/adapter/filesystem/fileinput"
)

// listReadBatchSize bounds temporary directory metadata, not the number of
// matching state records returned to the owner.
const listReadBatchSize = 128

// Store confines named state records to one pinned directory tree. It is safe
// for concurrent use and must be closed when its owning workbench exits.
type Store struct {
	mu       sync.Mutex
	boundary *os.Root
	rootName string
	root     *os.Root
	cleanup  goruntime.Cleanup
}

type openedRoots struct {
	boundary *os.Root
	root     *os.Root
}

// Open validates, creates, and pins an owned state root.
func Open(root string) (*Store, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("state directory is empty")
	}
	if !filepath.IsAbs(root) {
		return nil, errors.New("state directory must be absolute")
	}
	root = filepath.Clean(root)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve state directory: %w", err)
	}
	expected, err := os.Lstat(root)
	if err != nil {
		return nil, fmt.Errorf("inspect state directory: %w", err)
	}
	if !expected.IsDir() || expected.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("state root is not a directory")
	}
	boundaryName := filepath.Dir(root)
	rootName := filepath.Base(root)
	if boundaryName == root {
		rootName = "."
	}
	boundaryExpected, err := os.Stat(boundaryName)
	if err != nil {
		return nil, fmt.Errorf("inspect state boundary: %w", err)
	}
	boundary, err := os.OpenRoot(boundaryName)
	if err != nil {
		return nil, fmt.Errorf("open state boundary: %w", err)
	}
	boundaryOpened, err := boundary.Stat(".")
	if err != nil || !boundaryOpened.IsDir() || !os.SameFile(boundaryExpected, boundaryOpened) {
		_ = boundary.Close()
		if err != nil {
			return nil, fmt.Errorf("inspect opened state boundary: %w", err)
		}
		return nil, errors.New("state boundary changed while it was being opened")
	}
	pinned, err := boundary.OpenRoot(rootName)
	if err != nil {
		_ = boundary.Close()
		return nil, fmt.Errorf("open state directory: %w", err)
	}
	opened, err := pinned.Stat(".")
	if err != nil || !opened.IsDir() || !os.SameFile(expected, opened) {
		_ = pinned.Close()
		_ = boundary.Close()
		if err != nil {
			return nil, fmt.Errorf("inspect opened state directory: %w", err)
		}
		return nil, errors.New("state directory changed while it was being opened")
	}
	store := &Store{boundary: boundary, rootName: rootName, root: pinned}
	store.addCleanup()
	return store, nil
}

// Close releases the pinned state root. It is idempotent.
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.root == nil {
		return nil
	}
	roots := openedRoots{boundary: s.boundary, root: s.root}
	s.boundary = nil
	s.root = nil
	s.cleanup.Stop()
	goruntime.KeepAlive(s)
	return errors.Join(roots.root.Close(), roots.boundary.Close())
}

// Read returns one regular state file within the requested size bound.
func (s *Store) Read(name string, maximumBytes int64) ([]byte, error) {
	if maximumBytes <= 0 {
		return nil, errors.New("state read limit must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRoot(); err != nil {
		return nil, err
	}
	relative, err := stateName(name)
	if err != nil {
		return nil, err
	}
	directory, err := s.openDirectory(filepath.Dir(relative), false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = directory.Close() }()
	base := filepath.Base(relative)
	info, err := directory.Lstat(base)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("state file %q is not regular", name)
	}
	if info.Size() > maximumBytes {
		return nil, fmt.Errorf("state file %q exceeds %d bytes", name, maximumBytes)
	}
	file, _, err := fileinput.OpenAtExpected(directory, base, info, maximumBytes)
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

// ListFiles returns child file names with the requested extension in the order
// supplied by os.File.ReadDir.
func (s *Store) ListFiles(name, extension string) (_ []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRoot(); err != nil {
		return nil, err
	}
	directory, err := s.openDirectory(name, false)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	opened, _, err := fileinput.OpenDirectoryAt(directory, ".")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, opened.Close()) }()
	var names []string
	for {
		entries, readErr := opened.ReadDir(listReadBatchSize)
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == extension {
				names = append(names, entry.Name())
			}
		}
		switch {
		case errors.Is(readErr, io.EOF):
			return names, nil
		case readErr != nil:
			return nil, readErr
		}
	}
}

// Replace atomically publishes one complete state file. The temporary file is
// created beside the destination, so Rename cannot cross filesystems and
// intentionally replaces the prior snapshot instead of conflict-renaming it.
func (s *Store) Replace(name string, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRoot(); err != nil {
		return err
	}
	relative, err := stateName(name)
	if err != nil {
		return err
	}
	directory, err := s.openDirectory(filepath.Dir(relative), true)
	if err != nil {
		return err
	}
	defer func() { _ = directory.Close() }()
	temporary, temporaryName, err := createTemporary(directory)
	if err != nil {
		return fmt.Errorf("create state snapshot: %w", err)
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = directory.Remove(temporaryName)
		}
	}()
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
	if err := directory.Rename(temporaryName, filepath.Base(relative)); err != nil {
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
func (s *Store) Remove(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureRoot(); err != nil {
		return err
	}
	relative, err := stateName(name)
	if err != nil {
		return err
	}
	directory, err := s.openDirectory(filepath.Dir(relative), false)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer func() { _ = directory.Close() }()
	if err := directory.Remove(filepath.Base(relative)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	syncCommittedDirectory(directory)
	return nil
}

func (s *Store) ensureRoot() error {
	if s.root == nil || s.boundary == nil {
		return errors.New("state store is closed")
	}
	expected, err := s.boundary.Lstat(s.rootName)
	if err != nil {
		return err
	}
	if !expected.IsDir() || expected.Mode()&os.ModeSymlink != 0 {
		return errors.New("state root is not a directory")
	}
	opened, err := s.root.Stat(".")
	if err == nil && opened.IsDir() && os.SameFile(expected, opened) {
		return nil
	}
	replacement, err := s.boundary.OpenRoot(s.rootName)
	if err != nil {
		return fmt.Errorf("reopen state directory: %w", err)
	}
	rebound, err := replacement.Stat(".")
	if err != nil || !rebound.IsDir() || !os.SameFile(expected, rebound) {
		_ = replacement.Close()
		if err != nil {
			return fmt.Errorf("inspect reopened state directory: %w", err)
		}
		return errors.New("state directory changed while it was being reopened")
	}
	previous := s.root
	s.cleanup.Stop()
	goruntime.KeepAlive(s)
	s.root = replacement
	s.addCleanup()
	if err := previous.Close(); err != nil {
		return fmt.Errorf("close replaced state directory: %w", err)
	}
	return nil
}

func (s *Store) addCleanup() {
	roots := openedRoots{boundary: s.boundary, root: s.root}
	s.cleanup = goruntime.AddCleanup(s, func(roots openedRoots) {
		_ = roots.root.Close()
		_ = roots.boundary.Close()
	}, roots)
}

func stateName(name string) (string, error) {
	if name == "" || name == "." || filepath.IsAbs(name) || filepath.Clean(name) != name || strings.Contains(name, `\`) {
		return "", errors.New("state name must be a clean relative path")
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || !pathologize.IsClean(part) {
			return "", errors.New("state name contains a non-portable path segment")
		}
	}
	return filepath.Join(parts...), nil
}

func (s *Store) openDirectory(name string, create bool) (_ *os.Root, err error) {
	relative, expected, err := s.directory(name, create)
	if err != nil {
		return nil, err
	}
	directory, err := s.root.OpenRoot(relative)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, directory.Close())
		}
	}()
	opened, err := directory.Stat(".")
	if err != nil {
		return nil, err
	}
	if !opened.IsDir() || !os.SameFile(expected, opened) {
		return nil, errors.New("state directory changed while it was being opened")
	}
	return directory, nil
}

func (s *Store) directory(name string, create bool) (string, os.FileInfo, error) {
	if name == "." {
		info, err := s.root.Lstat(".")
		return ".", info, err
	}
	destination, err := stateName(name)
	if err != nil {
		return "", nil, err
	}
	current := ""
	var info os.FileInfo
	for _, part := range strings.Split(filepath.ToSlash(destination), "/") {
		parent := current
		if parent == "" {
			parent = "."
		}
		current = filepath.Join(current, part)
		info, err = s.root.Lstat(current)
		created := false
		if errors.Is(err, os.ErrNotExist) && create {
			created = true
			if mkdirErr := s.root.Mkdir(current, 0o700); mkdirErr != nil && !errors.Is(mkdirErr, os.ErrExist) {
				return "", nil, fmt.Errorf("create state directory: %w", mkdirErr)
			}
			info, err = s.root.Lstat(current)
		}
		if err != nil {
			return "", nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("state path %q contains a component that is not a directory", name)
		}
		if created {
			if syncErr := syncDirectoryAt(s.root, parent); syncErr != nil {
				return "", nil, fmt.Errorf("commit state directory: %w", syncErr)
			}
		}
	}
	return destination, info, nil
}

func createTemporary(directory *os.Root) (*os.File, string, error) {
	name := ".flame-state-" + rand.Text()
	file, err := directory.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	return file, name, err
}

func syncDirectoryAt(root *os.Root, name string) error {
	directory, _, err := fileinput.OpenDirectoryAt(root, name)
	if err != nil {
		return err
	}
	return errors.Join(directory.Sync(), directory.Close())
}

func syncCommittedDirectory(directory *os.Root) {
	_ = syncDirectoryAt(directory, ".")
}
