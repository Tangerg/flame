package http

import (
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/cors"
)

const corsPreflightCacheLifetime = 10 * time.Minute

func DefaultCORSOrigins() []string {
	return []string{
		"wails://localhost",      // Wails macOS/Linux asset server
		"http://wails.localhost", // Wails Windows asset server
		"http://127.0.0.1:5174",  // repository Wails/Vite dev task
		"http://localhost:5173",  // standalone Vite default
	}
}

func corsMiddleware(origins []string) func(http.Handler) http.Handler {
	if len(origins) == 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	opts := cors.Options{
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		// Browser tracing injects traceparent unconditionally, so omitting the trace
		// headers would fail preflight for every method.
		AllowedHeaders:   append(requestHeaders(), "Accept", "traceparent", "tracestate", "baggage"),
		ExposedHeaders:   []string{headerRequestID, headerServer, headerMethod},
		AllowCredentials: true,
		MaxAge:           int(corsPreflightCacheLifetime / time.Second),
	}
	// With credentials the CORS spec forbids a literal "*", so allow-all reflects the
	// request origin instead.
	if slices.Contains(origins, "*") {
		opts.AllowOriginFunc = func(*http.Request, string) bool { return true }
	} else {
		opts.AllowedOrigins = origins
	}
	return cors.Handler(opts)
}
