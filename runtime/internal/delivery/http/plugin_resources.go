package http

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
)

// NewPluginResourceHandler projects a subscription-lived Core query as an
// ordinary module/asset URL. No persistent bearer credential appears in it.
func NewPluginResourceHandler(endpoint *delivery.Endpoint) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		remaining, ok := strings.CutPrefix(r.URL.Path, pluginResourcePrefix)
		token, resource, found := strings.Cut(remaining, "/")
		if !ok || !found || token == "" || resource == "" || r.Method != http.MethodGet || r.URL.RawQuery != "" {
			http.Error(w, "plugin resource is unavailable", http.StatusGone)
			return
		}
		value, err := endpoint.LeasedPluginResource(r.Context(), token, resource)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, protocol.ErrPluginUnavailable) || errors.Is(err, protocol.ErrPluginDisabled) || errors.Is(err, protocol.ErrPluginStale) || errors.Is(err, protocol.ErrPluginNotFound) || errors.Is(err, protocol.ErrInvalidParams) {
				status = http.StatusGone
			}
			http.Error(w, "plugin resource is unavailable", status)
			return
		}
		body, err := base64.StdEncoding.DecodeString(value.Body)
		if err != nil {
			http.Error(w, "plugin resource could not be decoded", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", value.MediaType)
		_, _ = w.Write(body)
	})
}
