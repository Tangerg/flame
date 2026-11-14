package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Tangerg/flame/packages/local-runtime/webassets"
)

func newWebAssets(directory string) (*webassets.Handler, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("webassets: web directory must be absolute")
	}
	// Keep filesystem authority beneath the configured deployment root, including
	// symlinks and concurrent path replacements.
	return webassets.New(func(name string) (fs.File, error) {
		return os.OpenInRoot(directory, name)
	})
}
