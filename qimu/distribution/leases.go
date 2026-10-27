package distribution

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"slices"

	"github.com/Tangerg/flame/qimu"
)

func (s *Store) InstallDirectory(ctx context.Context, directory string) (Release, error) {
	lease, err := s.InstallDirectoryHeld(ctx, directory)
	if err != nil {
		return Release{}, err
	}
	return lease.Release(), lease.Close()
}

func (s *Store) InstallArchive(ctx context.Context, fileName string, expected qimu.Digest) (Release, error) {
	lease, err := s.InstallArchiveHeld(ctx, fileName, expected)
	if err != nil {
		return Release{}, err
	}
	return lease.Release(), lease.Close()
}

// Release is a detached immutable declaration projection. Retaining it does not
// retain content; the original Lease owns that claim until Close.
func (l *Lease) Release() Release {
	l.mu.Lock()
	defer l.mu.Unlock()
	encoded, _ := json.Marshal(l.release)
	var result Release
	_ = json.Unmarshal(encoded, &result)
	return result
}

// Directory permits a trusted profile adapter to open the exact admitted tree.
// Keep this lease until every object that may use the directory has exited.
// Directory is not a writable data root or proof that a process has exited.
func (l *Lease) Directory() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return "", qimu.ErrClosed
	}
	return l.store.Path(l.release.Digest)
}

// ReadLimit applies a consumer's smaller resource budget before allocating the
// admitted artifact. Content validation remains owned by the Store.
func (l *Lease) ReadLimit(ctx context.Context, name string, maximum int64) ([]byte, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, qimu.ErrClosed
	}
	if maximum < 0 {
		return nil, ErrInvalid
	}
	for _, artifact := range l.release.Manifest.Artifacts {
		if artifact.Path == name {
			if artifact.Size > maximum {
				return nil, ErrCapacity
			}
			return l.store.readArtifact(ctx, l.release, name)
		}
	}
	return nil, ErrInvalid
}

// Path projects an immutable release location for diagnostics and declarations.
// It performs no admission: actual reads and execution require an acquired Lease.
func (s *Store) Path(digest qimu.Digest) (string, error) {
	done, err := s.beginOperation(true)
	if err != nil {
		return "", err
	}
	defer done()
	if digest.Validate() != nil {
		return "", ErrInvalid
	}
	return filepath.Join(s.directory, string(digest)), nil
}

// Digests observes the content namespace. Each later Remove still checks the
// original retention facts and live claims under the shared admission gate.
func (s *Store) Digests(ctx context.Context) (_ []qimu.Digest, err error) {
	done, err := s.beginOperation(false)
	if err != nil {
		return nil, err
	}
	defer done()
	directory, err := s.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, directory.Close()) }()
	var result []qimu.Digest
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, readErr := directory.ReadDir(256)
		for _, entry := range entries {
			if digest, err := qimu.ParseDigest(entry.Name()); err == nil {
				result = append(result, digest)
			}
		}
		if errors.Is(readErr, io.EOF) {
			slices.Sort(result)
			return result, nil
		}
		if readErr != nil {
			return nil, readErr
		}
	}
}
