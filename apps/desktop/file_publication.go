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
	return &exportFile{file: file, destination: destination, mode: mode}, nil
}

func (f *exportFile) discard() {
	if f.file == nil {
		return
	}
	_ = f.file.Close()
	_ = os.Remove(f.file.Name())
	f.file = nil
}

// Rename is the only publication point, including for exports delivered in chunks.
func (f *exportFile) publish() error {
	if err := f.file.Chmod(f.mode); err != nil {
		return err
	}
	if err := errors.Join(f.file.Sync(), f.file.Close()); err != nil {
		return err
	}
	if err := os.Rename(f.file.Name(), f.destination); err != nil {
		return fmt.Errorf("publish export: %w", err)
	}
	f.file = nil
	return nil
}
func publishExport(destination string, content io.Reader) error {
	f, err := prepareExport(destination)
	if err != nil {
		return err
	}
	defer f.discard()
	if _, err := io.Copy(f.file, content); err != nil {
		return err
	}
	return f.publish()
}
