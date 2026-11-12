package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type exportFile struct {
	file        *os.File
	destination string
	mode        os.FileMode
	// Closing revokes writes; the staging path stays owned until rename or removal is confirmed.
	stagingPath string
}

func prepareExport(destination string) (*exportFile, error) {
	mode := os.FileMode(0600)
	previous, err := os.Lstat(destination)
	switch {
	case err == nil:
		if !previous.Mode().IsRegular() {
			return nil, errors.New("export destination is not a regular file")
		}
		mode = previous.Mode().Perm()
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".flame-export-*")
	if err != nil {
		return nil, err
	}
	return &exportFile{file: file, destination: destination, mode: mode, stagingPath: file.Name()}, nil
}

func (f *exportFile) Write(contents []byte) (int, error) {
	if f.file == nil {
		return 0, os.ErrClosed
	}
	return f.file.Write(contents)
}

func (f *exportFile) position() (int64, error) {
	if f.file == nil {
		return 0, os.ErrClosed
	}
	return f.file.Seek(0, io.SeekCurrent)
}

func (f *exportFile) close() error {
	if f.file == nil {
		return nil
	}
	file := f.file
	f.file = nil
	return file.Close()
}

func (f *exportFile) ownsStagingFile() bool { return f.stagingPath != "" }

func (f *exportFile) discard() error {
	closed := f.close()
	if !f.ownsStagingFile() {
		return closed
	}
	removed := os.Remove(f.stagingPath)
	if removed == nil || errors.Is(removed, os.ErrNotExist) {
		f.stagingPath = ""
		removed = nil
	}
	return errors.Join(closed, removed)
}

// Rename is the only publication point, including for exports delivered in chunks.
func (f *exportFile) publish() error {
	if f.file == nil {
		return os.ErrClosed
	}
	if err := f.file.Chmod(f.mode); err != nil {
		return err
	}
	if err := errors.Join(f.file.Sync(), f.close()); err != nil {
		return err
	}
	if err := os.Rename(f.stagingPath, f.destination); err != nil {
		return fmt.Errorf("publish export: %w", err)
	}
	f.stagingPath = ""
	return nil
}
func publishExport(destination string, content io.Reader) (returned error) {
	f, err := prepareExport(destination)
	if err != nil {
		return err
	}
	defer func() { returned = errors.Join(returned, f.discard()) }()
	if _, err := io.Copy(f, content); err != nil {
		return err
	}
	return f.publish()
}
