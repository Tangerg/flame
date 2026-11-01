package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// A failed export must leave the user's previous file intact. Rename is the
// commit point, so every fallible preparation happens in its directory first.
func publishExport(destination string, content io.Reader) error {
	mode := os.FileMode(0600)
	previous, err := os.Lstat(destination)
	switch {
	case err == nil:
		if !previous.Mode().IsRegular() {
			return errors.New("export destination is not a regular file")
		}
		mode = previous.Mode().Perm()
	case !errors.Is(err, os.ErrNotExist):
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".flame-export-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temporary.Name()) }()
	if _, err := io.Copy(temporary, content); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := temporary.Chmod(mode); err != nil {
		return errors.Join(err, temporary.Close())
	}
	if err := errors.Join(temporary.Sync(), temporary.Close()); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), destination); err != nil {
		return fmt.Errorf("publish export: %w", err)
	}
	return nil
}
