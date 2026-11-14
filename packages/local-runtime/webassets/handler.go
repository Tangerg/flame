// Package webassets delivers one built Web distribution to HTTP and native hosts.
package webassets

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

const policyFile = ".vite/flame-web-policy.json"

type artifact struct {
	SHA256      string `json:"sha256"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Document    string `json:"document,omitempty"`
}

type policy struct {
	Version          int    `json:"version"`
	NoncePlaceholder string `json:"noncePlaceholder"`
	DocumentPolicy   string `json:"documentPolicy"`
	EmbeddedPolicy   string `json:"embeddedPolicy"`
	Sandbox          struct {
		File   string `json:"file"`
		Policy string `json:"policy"`
	} `json:"sandbox"`
	Files map[string]artifact `json:"files"`
}

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
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&h.policy); err != nil {
		return nil, fmt.Errorf("webassets: decode artifact policy: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, errors.New("webassets: trailing artifact policy data")
	}
	p := h.policy
	if p.Version != 1 || p.NoncePlaceholder == "" || strings.ContainsAny(p.NoncePlaceholder, "\r\n'\"<>&") ||
		!strings.Contains(p.DocumentPolicy, "'nonce-"+p.NoncePlaceholder+"'") ||
		!strings.Contains(p.EmbeddedPolicy, "'nonce-"+p.NoncePlaceholder+"'") ||
		strings.ContainsAny(p.DocumentPolicy+p.EmbeddedPolicy+p.Sandbox.Policy, "\r\n") ||
		!publicPath(p.Sandbox.File) || path.Ext(p.Sandbox.File) != ".html" || p.Sandbox.Policy == "" {
		return nil, errors.New("webassets: invalid artifact policy")
	}
	for name, file := range p.Files {
		hash, err := hex.DecodeString(file.SHA256)
		if !publicPath(name) || err != nil || len(hash) != sha256.Size ||
			(file.Fingerprint != "" && file.Fingerprint != "sha256" && file.Fingerprint != "bundler") {
			return nil, errors.New("webassets: invalid artifact identity")
		}
		if file.Fingerprint == "sha256" && path.Base(name) != file.SHA256+path.Ext(name) {
			return nil, errors.New("webassets: invalid content-addressed asset")
		}
		if path.Ext(name) == ".html" {
			if file.Document != "main" && file.Document != "same-origin" && file.Document != "sandbox" {
				return nil, errors.New("webassets: invalid document kind")
			}
			if (file.Document == "sandbox") != (name == p.Sandbox.File) {
				return nil, errors.New("webassets: invalid sandbox identity")
			}
		} else if file.Document != "" {
			return nil, errors.New("webassets: document kind on a non-document artifact")
		}
	}
	if p.Files["index.html"].Document != "main" {
		return nil, errors.New("webassets: the application entry must be a main document")
	}
	for _, name := range []string{"index.html", p.Sandbox.File} {
		data, _, err := h.read(name)
		if err != nil {
			return nil, fmt.Errorf("webassets: read required document: %w", err)
		}
		if !h.matches(name, data) {
			return nil, errors.New("webassets: required document differs from its artifact policy")
		}
		if name == "index.html" && !bytes.Contains(data, []byte(p.NoncePlaceholder)) {
			return nil, errors.New("webassets: document has no response nonce placeholder")
		}
	}
	return h, nil
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
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, errors.Join(err, file.Close(), errors.New("not a regular artifact"))
	}
	data, readErr := io.ReadAll(file)
	return data, info, errors.Join(readErr, file.Close())
}

func (h *Handler) matches(name string, data []byte) bool {
	file, ok := h.policy.Files[name]
	if !ok {
		return false
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]) == file.SHA256
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
		name = "index.html"
	}
	if !publicPath(name) {
		http.NotFound(w, r)
		return
	}
	data, info, err := h.read(name)
	if errors.Is(err, fs.ErrNotExist) && path.Ext(name) == "" && strings.Contains(r.Header.Get("Accept"), "text/html") {
		name = "index.html"
		data, info, err = h.read(name)
	}
	if err != nil {
		http.NotFound(w, r)
		return
	}
	identity, known := h.policy.Files[name]
	if known && !h.matches(name, data) {
		http.Error(w, "web artifact integrity check failed", http.StatusServiceUnavailable)
		return
	}
	if path.Ext(name) == ".html" {
		if !known {
			http.NotFound(w, r)
			return
		}
		if name == h.policy.Sandbox.File {
			w.Header().Set("Content-Security-Policy", h.policy.Sandbox.Policy)
		} else {
			if !bytes.Contains(data, []byte(h.policy.NoncePlaceholder)) {
				http.Error(w, "web document has no response nonce placeholder", http.StatusServiceUnavailable)
				return
			}
			nonce := make([]byte, 24)
			if _, err := rand.Read(nonce); err != nil {
				http.Error(w, "web document nonce unavailable", http.StatusInternalServerError)
				return
			}
			value := base64.StdEncoding.EncodeToString(nonce)
			data = bytes.ReplaceAll(data, []byte(h.policy.NoncePlaceholder), []byte(value))
			csp := h.policy.DocumentPolicy
			xfo := "DENY"
			if identity.Document == "same-origin" {
				csp = h.policy.EmbeddedPolicy
				xfo = "SAMEORIGIN"
			}
			w.Header().Set("Content-Security-Policy", strings.ReplaceAll(csp, h.policy.NoncePlaceholder, value))
			w.Header().Set("X-Frame-Options", xfo)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		// A fresh nonce cannot be paired with a cached body through a 304.
		w.WriteHeader(http.StatusOK)
		if r.Method != http.MethodHead {
			_, _ = w.Write(data)
		}
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
