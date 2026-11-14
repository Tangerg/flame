// Package webassets delivers one built Web distribution to HTTP and native hosts.
package webassets

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

type Handler struct {
	open   func(string) (fs.File, error)
	policy policy
}

// New captures the build's policy. Each response verifies the bytes it serves;
// a mutable directory never turns a stale manifest into an immutable response.
func New(open func(string) (fs.File, error)) (*Handler, error) {
	if open == nil {
		return nil, errors.New("webassets: missing artifact source")
	}
	h := &Handler{open: open}
	data, _, err := h.read(policyFile)
	if err != nil {
		return nil, fmt.Errorf("webassets: read artifact policy: %w", err)
	}
	h.policy, err = parsePolicy(data)
	if err != nil {
		return nil, err
	}
	if err := h.verifyRequiredDocuments(); err != nil {
		return nil, err
	}
	return h, nil
}

func (h *Handler) verifyRequiredDocuments() error {
	for _, name := range []string{entryDocument, h.policy.Sandbox.File} {
		data, _, err := h.read(name)
		if err != nil {
			return fmt.Errorf("webassets: read required document: %w", err)
		}
		if !h.policy.Files[name].matches(data) {
			return errors.New("webassets: required document differs from its artifact policy")
		}
		if name == entryDocument && !bytes.Contains(data, []byte(h.policy.NoncePlaceholder)) {
			return errors.New("webassets: document has no response nonce placeholder")
		}
	}
	return nil
}

func publicPath(name string) bool {
	if !fs.ValidPath(name) || name == "." || strings.Contains(name, "\\") {
		return false
	}
	for _, component := range strings.Split(name, "/") {
		if strings.HasPrefix(component, ".") {
			return false
		}
	}
	return true
}

func (h *Handler) read(name string) ([]byte, fs.FileInfo, error) {
	file, err := h.open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, nil, errors.Join(err, file.Close())
	}
	if !info.Mode().IsRegular() {
		if err := file.Close(); err != nil {
			return nil, nil, err
		}
		return nil, nil, errors.New("not a regular artifact")
	}
	data, readErr := io.ReadAll(file)
	return data, info, errors.Join(readErr, file.Close())
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = entryDocument
	}
	if !publicPath(name) {
		http.NotFound(w, r)
		return
	}
	data, info, err := h.read(name)
	if errors.Is(err, fs.ErrNotExist) && path.Ext(name) == "" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		name = entryDocument
		data, info, err = h.read(name)
	}
	identity, known := h.policy.Files[name]
	if err != nil {
		// Unpublished lookups must not reveal a refused source's existence.
		if errors.Is(err, fs.ErrNotExist) || !known {
			http.NotFound(w, r)
		} else {
			http.Error(w, "web artifact unavailable", http.StatusServiceUnavailable)
		}
		return
	}
	if known && !identity.matches(data) {
		http.Error(w, "web artifact integrity check failed", http.StatusServiceUnavailable)
		return
	}
	if path.Ext(name) == ".html" {
		if !known {
			http.NotFound(w, r)
			return
		}
		h.serveDocument(w, r, identity.Document, data)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if known && identity.Fingerprint != "" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("ETag", `"`+identity.SHA256+`"`)
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(data))
}
