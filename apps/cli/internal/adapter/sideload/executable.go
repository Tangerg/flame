package sideload

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/go-sdk/filesystem"
)

type executableSource struct {
	path     string
	identity os.FileInfo
	digest   []byte
}

func captureExecutable(directory, relative string) (executableSource, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return executableSource{}, err
	}
	defer func() { _ = root.Close() }()
	path := filepath.Join(directory, relative)
	file, identity, err := filesystem.OpenFileAt(root, relative, 0)
	if err != nil {
		return executableSource{}, err
	}
	defer func() { _ = file.Close() }()
	if err := validateExecutable(path, identity); err != nil {
		return executableSource{}, err
	}
	hash := sha256.New()
	if _, err := io.CopyN(hash, file, identity.Size()); err != nil {
		return executableSource{}, fmt.Errorf("read executable: %w", err)
	}
	return executableSource{path: path, identity: identity, digest: hash.Sum(nil)}, nil
}

// Executing the source path would reopen it after admission. The private copy
// pins the exact bytes for both native images and interpreter-backed scripts.
func (s executableSource) stage(directory string) (string, error) {
	source, _, err := filesystem.OpenFileExpected(s.path, s.identity, 0)
	if errors.Is(err, filesystem.ErrChanged) {
		return "", fmt.Errorf("entry changed since discovery: %w", err)
	}
	if err != nil {
		return "", err
	}
	defer func() { _ = source.Close() }()
	path := filepath.Join(directory, filepath.Base(s.path))
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, copyErr := io.CopyN(io.MultiWriter(target, hash), source, s.identity.Size())
	if err := errors.Join(copyErr, target.Close()); err != nil {
		return "", fmt.Errorf("stage executable: %w", err)
	}
	if !bytes.Equal(s.digest, hash.Sum(nil)) {
		return "", errors.New("entry changed since discovery")
	}
	return path, nil
}
