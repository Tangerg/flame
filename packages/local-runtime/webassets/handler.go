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
	for _, name := range []string{entryDocument} {
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
	w.Header().Set(headerCacheControl, cacheNever)
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
	name, data, info, err := h.readRequested(name, r.Header.Get(headerAccept))
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
	if isDocument(name) {
		if !known {
			http.NotFound(w, r)
			return
		}
		h.serveDocument(w, r, identity.Document, data)
		return
	}
	setAssetHeaders(w.Header(), name, identity, known)
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(data))
}

const (
	headerAccept       = "Accept"
	headerCacheControl = "Cache-Control"
	headerContentType  = "Content-Type"
	headerETag         = "ETag"

	cacheNever      = "no-store"
	cacheRevalidate = "no-cache"
	cacheImmutable  = "public, max-age=31536000, immutable"
)

func (h *Handler) readRequested(name, accept string) (string, []byte, fs.FileInfo, error) {
	data, info, err := h.read(name)
	if errors.Is(err, fs.ErrNotExist) && path.Ext(name) == "" && strings.Contains(accept, "text/html") {
		name = entryDocument
		data, info, err = h.read(name)
	}
	return name, data, info, err
}

func setAssetHeaders(header http.Header, name string, identity artifact, known bool) {
	header.Set(headerCacheControl, cacheRevalidate)
	if known && identity.Fingerprint != "" {
		header.Set(headerCacheControl, cacheImmutable)
		header.Set(headerETag, `"`+identity.SHA256+`"`)
	}
	if contentType := mime.TypeByExtension(path.Ext(name)); contentType != "" {
		header.Set(headerContentType, contentType)
	}
}
