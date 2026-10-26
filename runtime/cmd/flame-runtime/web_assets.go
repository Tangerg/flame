package main

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type webAssets struct {
	directory string
}

func newWebAssets(directory string) (*webAssets, error) {
	if !filepath.IsAbs(directory) {
		return nil, errors.New("webassets: web directory must be absolute")
	}
	index, err := os.OpenInRoot(directory, "index.html")
	if err != nil {
		return nil, fmt.Errorf("webassets: open web application: %w", err)
	}
	info, statErr := index.Stat()
	closeErr := index.Close()
	if err := errors.Join(statErr, closeErr); err != nil {
		return nil, fmt.Errorf("webassets: inspect web application: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("webassets: web index must be a regular file")
	}
	return &webAssets{directory: directory}, nil
}

func (h *webAssets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	// URL paths never acquire host-specific separators or hidden files.
	// OpenInRoot additionally confines symlinks and concurrent path changes.
	if !fs.ValidPath(name) || strings.Contains(name, "\\") {
		http.NotFound(w, r)
		return
	}
	for _, component := range strings.Split(name, "/") {
		if strings.HasPrefix(component, ".") {
			http.NotFound(w, r)
			return
		}
	}
	file, err := os.OpenInRoot(h.directory, name)
	if errors.Is(err, fs.ErrNotExist) && path.Ext(name) == "" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		name = "index.html"
		file, err = os.OpenInRoot(h.directory, name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, info.ModTime(), file)
}
