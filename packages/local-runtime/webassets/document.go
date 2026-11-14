package webassets

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
)

const responseNonceBytes = 24

func (h *Handler) serveDocument(w http.ResponseWriter, r *http.Request, kind documentKind, data []byte) {
	if kind == sandboxDocument {
		w.Header().Set("Content-Security-Policy", h.policy.Sandbox.Policy)
	} else {
		if !bytes.Contains(data, []byte(h.policy.NoncePlaceholder)) {
			http.Error(w, "web document has no response nonce placeholder", http.StatusServiceUnavailable)
			return
		}
		nonce := make([]byte, responseNonceBytes)
		if _, err := rand.Read(nonce); err != nil {
			http.Error(w, "web document nonce unavailable", http.StatusInternalServerError)
			return
		}
		value := base64.StdEncoding.EncodeToString(nonce)
		data = bytes.ReplaceAll(data, []byte(h.policy.NoncePlaceholder), []byte(value))
		csp := h.policy.DocumentPolicy
		xfo := "DENY"
		if kind == sameOriginDocument {
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
}
